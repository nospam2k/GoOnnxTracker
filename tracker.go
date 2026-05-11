package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Tracker struct {
	inference *Inference
	hub       *Hub
	frameCh   chan []byte

	ctx       context.Context
	cancel    context.CancelFunc
	camURL    string
	lastDir   string
	lastBox   *Box
	httpClient *http.Client
	fetchSem  chan struct{}
}

func (t *Tracker) Run() {
	t.ctx, t.cancel = context.WithCancel(context.Background())
	defer t.cancel()
	t.httpClient = &http.Client{Timeout: 3 * time.Second}
	t.fetchSem = make(chan struct{}, 1)

	ticker := time.NewTicker(33 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case frame := <-t.frameCh:
			if len(frame) > 0 {
				t.processFrame(frame)
			}
		case <-ticker.C:
			select {
			case t.fetchSem <- struct{}{}:
				go t.fetchFrameWithSem()
			default:
			}
		}
	}
}

func (t *Tracker) fetchFrameWithSem() {
	defer func() { <-t.fetchSem }()
	t.fetchFrame()
}

func (t *Tracker) fetchFrame() {
	mu.RLock()
	url := cameraURL
	mu.RUnlock()

	if url == "" {
		return
	}

	ctx, cancel := context.WithTimeout(t.ctx, 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	var frame []byte
	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart") {
		frame = extractJPEGFromMJPEG(resp.Body)
	} else {
		var buf bytes.Buffer
		buf.ReadFrom(resp.Body)
		frame = buf.Bytes()
	}

	if len(frame) > 0 {
		select {
		case t.frameCh <- frame:
		default:
		}
	}
}

func extractJPEGFromMJPEG(r interface {
	Read([]byte) (int, error)
}) []byte {
	reader := bufio.NewReader(r)
	var contentLen int

	// Read headers until we find Content-Length or a blank line
	for {
		line, _ := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" {
			break
		}
		if strings.Contains(line, "Content-Length") {
			var n int
			fmt.Sscanf(line, "Content-Length: %d", &n)
			contentLen = n
		}
	}

	// Read JPEG data
	if contentLen > 0 {
		b := make([]byte, contentLen)
		n, _ := reader.Read(b)
		return b[:n]
	}

	// Fallback: read until we find JPEG end marker
	var jpegData bytes.Buffer
	for {
		b := make([]byte, 4096)
		n, _ := reader.Read(b)
		if n == 0 {
			break
		}
		jpegData.Write(b[:n])
		if bytes.Contains(jpegData.Bytes(), []byte{0xFF, 0xD9}) {
			idx := bytes.LastIndex(jpegData.Bytes(), []byte{0xFF, 0xD9})
			return jpegData.Bytes()[:idx+2]
		}
	}
	return nil
}

func (t *Tracker) processFrame(jpegData []byte) {
	detection, err := t.inference.Infer(jpegData)
	if err != nil {
		t.hub.Broadcast <- NotFoundJSON()
		return
	}

	if detection == nil {
		t.hub.Broadcast <- NotFoundJSON()
		t.handleTrackingLost()
		return
	}

	t.hub.Broadcast <- detection.ToJSON()
	t.handleTracking(detection)
}

func (t *Tracker) handleTracking(detection *Detection) {
	mu.RLock()
	ip := extractIPFromURL(cameraURL)
	mu.RUnlock()

	if ip == "" {
		return
	}

	data, err := os.ReadFile(camerasFile)
	if err != nil {
		return
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return
	}

	// Find the slot for the current camera
	slot := findCameraSlot(config, ip)

	if !getBool(config, "trackingActive", slot) {
		return
	}

	settings := getSettings(config, "trackingSettings", slot)
	if settings == nil {
		return
	}

	deadZone, ok := settings["deadZone"].(map[string]interface{})
	if !ok {
		return
	}

	left := getFloat(deadZone, "left", 0.3)
	right := getFloat(deadZone, "right", 0.7)
	maxSpeed := getInt(config, "trackingMaxSpeed", slot, 10)

	box := detection.Box
	boxCenter := box.X + box.W/2
	deadZoneCenter := (left + right) / 2
	deadZoneHalfWidth := (right - left) / 2

	offset := boxCenter - deadZoneCenter // negative = subject left of center

	if t.lastDir == "" {
		// Currently stopped — only start if outside the dead zone
		if boxCenter >= left && boxCenter <= right {
			t.lastBox = &box
			return
		}
	} else {
		// Currently moving — stop once subject is within the dead zone
		if boxCenter >= left && boxCenter <= right {
			t.lastDir = ""
			t.sendCGI(ip, "/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
			t.lastBox = &box
			return
		}
		// Also stop if we've overshot past the far edge of the frame
		// (moved right while subject was left, or vice versa — true overshoot)
		if (offset < 0 && boxCenter <= 0) || (offset > 0 && boxCenter >= 1) {
			t.lastDir = ""
			t.sendCGI(ip, "/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
			t.lastBox = &box
			return
		}
	}

	// Determine direction
	var dir string
	if offset < 0 {
		dir = "left"
	} else {
		dir = "right"
	}

	// Proportional speed: scale linearly from minSpeed at the dead zone edge
	// up to maxSpeed at the frame edge (0 or 1).
	minSpeed := 2
	if minSpeed > maxSpeed {
		minSpeed = maxSpeed
	}
	distFromCenter := offset
	if distFromCenter < 0 {
		distFromCenter = -distFromCenter
	}
	// Normalise: 0 at dead zone edge, 1 at frame edge
	normalised := (distFromCenter - deadZoneHalfWidth) / (0.5 - deadZoneHalfWidth)
	if normalised < 0 {
		normalised = 0
	}
	if normalised > 1 {
		normalised = 1
	}
	speed := minSpeed + int(normalised*float64(maxSpeed-minSpeed))

	// Only send a new CGI command when direction or speed bucket changes,
	// to avoid hammering the camera with identical commands every frame.
	speedBucket := speed / 2 // group into buckets of 2 to reduce chatter
	stateKey := fmt.Sprintf("%s-%d", dir, speedBucket)
	if stateKey != t.lastDir {
		t.lastDir = stateKey
		cmd := fmt.Sprintf("/cgi-bin/ptzctrl.cgi?ptzcmd&%s&%d&%d", dir, speed, speed)
		t.sendCGI(ip, cmd)
	}
	t.lastBox = &box
}

func (t *Tracker) handleTrackingLost() {
	if t.lastDir != "" {
		t.lastDir = ""
		mu.RLock()
		ip := extractIPFromURL(cameraURL)
		mu.RUnlock()
		if ip != "" {
			t.sendCGI(ip, "/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
		}
	}
	t.lastBox = nil
}

func (t *Tracker) sendCGI(ip, cgiPath string) {
	if t.httpClient == nil {
		return
	}
	go func() {
		target := "http://" + ip + cgiPath
		_, err := t.httpClient.Get(target)
		if err != nil {
			log.Printf("[Tracking] CGI error: %v", err)
		}
	}()
}

func extractIPFromURL(url string) string {
	parts := strings.Split(url, "//")
	if len(parts) < 2 {
		return ""
	}
	parts = strings.Split(parts[1], "/")
	return parts[0]
}

func getBool(config map[string]interface{}, key string, slot int) bool {
	field, ok := config[key].(map[string]interface{})
	if !ok {
		return false
	}
	slotKey := fmt.Sprintf("%d", slot)
	val, ok := field[slotKey].(bool)
	return ok && val
}

func getInt(config map[string]interface{}, key string, slot int, def int) int {
	field, ok := config[key].(map[string]interface{})
	if !ok {
		return def
	}
	slotKey := fmt.Sprintf("%d", slot)
	val, ok := field[slotKey].(float64)
	if ok {
		return int(val)
	}
	return def
}

func getSettings(config map[string]interface{}, key string, slot int) map[string]interface{} {
	field, ok := config[key].(map[string]interface{})
	if !ok {
		return nil
	}
	slotKey := fmt.Sprintf("%d", slot)
	settings, ok := field[slotKey].(map[string]interface{})
	if ok {
		return settings
	}
	return nil
}

func getFloat(m map[string]interface{}, key string, def float64) float64 {
	val, ok := m[key].(float64)
	if ok {
		return val
	}
	return def
}

func findCameraSlot(config map[string]interface{}, ip string) int {
	cameras, ok := config["cameras"].([]interface{})
	if !ok {
		return 0
	}
	for i, cam := range cameras {
		if camMap, ok := cam.(map[string]interface{}); ok {
			if camIP, ok := camMap["ip"].(string); ok && camIP == ip {
				return i
			}
		}
	}
	return 0
}
