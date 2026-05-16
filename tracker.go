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
	"sync"
	"sync/atomic"
	"time"
)

// cachedConfig holds the parsed cameras.json so we don't hit disk every frame.
type cachedConfig struct {
	mu      sync.RWMutex
	data    map[string]interface{}
	modTime time.Time
}

func (c *cachedConfig) load(path string) map[string]interface{} {
	// Check file mod time — only re-read if changed.
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	c.mu.RLock()
	if !info.ModTime().After(c.modTime) && c.data != nil {
		defer c.mu.RUnlock()
		return c.data
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if !info.ModTime().After(c.modTime) && c.data != nil {
		return c.data
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return c.data // return stale on error
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return c.data
	}
	c.data = cfg
	c.modTime = info.ModTime()
	return c.data
}

var configCache cachedConfig

// notFoundMsg is a pre-marshaled constant — no allocation per frame.
var notFoundMsg = func() []byte {
	b, _ := json.Marshal(map[string]string{"cmd": "notfound"})
	return b
}()

type Tracker struct {
	inference *Inference
	hub       *Hub
	frameCh   chan []byte

	ctx    context.Context
	cancel context.CancelFunc

	lastDir string
	lastBox *Box

	httpClient *http.Client
	fetchSem   chan struct{}

	// CGI command deduplication: serialize PTZ commands through a single
	// buffered channel so we never pile up goroutines.
	cgiCh chan string

	// lastBroadcast tracks the last payload sent so we skip identical frames.
	lastBroadcast atomic.Pointer[[]byte]
}

func (t *Tracker) Run() {
	t.ctx, t.cancel = context.WithCancel(context.Background())
	defer t.cancel()

	t.httpClient = &http.Client{Timeout: 3 * time.Second}
	t.fetchSem = make(chan struct{}, 1)
	t.cgiCh = make(chan string, 4)

	// Single CGI worker — no goroutine pile-up.
	go t.cgiWorker()

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

// cgiWorker drains cgiCh and sends HTTP requests serially, dropping
// redundant commands that arrive while a request is in-flight.
func (t *Tracker) cgiWorker() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case cmd := <-t.cgiCh:
			ip := GetCameraIP()
			if ip == "" {
				continue
			}
			target := "http://" + ip + cmd
			resp, err := t.httpClient.Get(target)
			if err != nil {
				log.Printf("[Tracking] CGI error: %v", err)
				continue
			}
			resp.Body.Close()
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

	for {
		line, err := reader.ReadString('\n')
		if line == "\r\n" || line == "\n" || err != nil {
			break
		}
		if strings.HasPrefix(line, "Content-Length") {
			fmt.Sscanf(line, "Content-Length: %d", &contentLen)
		}
	}

	if contentLen > 0 {
		b := make([]byte, contentLen)
		n, _ := reader.Read(b)
		return b[:n]
	}

	// Fallback: scan for JPEG end marker 0xFF 0xD9.
	// Read into a single growing buffer and stop as soon as we find it
	// rather than scanning the whole buffer from the start each iteration.
	var jpegData bytes.Buffer
	chunk := make([]byte, 4096)
	for {
		n, _ := reader.Read(chunk)
		if n == 0 {
			break
		}
		jpegData.Write(chunk[:n])
		// Only scan the tail — the marker must end at or after the new data.
		tail := jpegData.Bytes()
		start := jpegData.Len() - n - 1
		if start < 0 {
			start = 0
		}
		if idx := bytes.Index(tail[start:], []byte{0xFF, 0xD9}); idx >= 0 {
			end := start + idx + 2
			result := make([]byte, end)
			copy(result, tail[:end])
			return result
		}
	}
	return nil
}

func (t *Tracker) processFrame(jpegData []byte) {
	detection, err := t.inference.Infer(jpegData)
	if err != nil || detection == nil {
		// Only broadcast notfound if we previously had a detection,
		// to avoid flooding the websocket with identical messages.
		prev := t.lastBroadcast.Load()
		if prev == nil || string(*prev) != string(notFoundMsg) {
			t.hub.Broadcast <- notFoundMsg
			t.lastBroadcast.Store(&notFoundMsg)
		}
		if detection == nil {
			t.handleTrackingLost()
		}
		return
	}

	payload := detection.ToJSON()

	// Skip broadcast if payload is identical to last frame.
	prev := t.lastBroadcast.Load()
	if prev == nil || string(*prev) != string(payload) {
		t.hub.Broadcast <- payload
		t.lastBroadcast.Store(&payload)
	}

	t.handleTracking(detection)
}

func (t *Tracker) handleTracking(detection *Detection) {
	// Use cached config — no disk I/O on the hot path.
	config := configCache.load(camerasFile)
	if config == nil {
		return
	}

	ip := GetCameraIP()
	if ip == "" {
		return
	}

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

	offset := boxCenter - deadZoneCenter

	if t.lastDir == "" {
		if boxCenter >= left && boxCenter <= right {
			t.lastBox = &box
			return
		}
	} else {
		if boxCenter >= left && boxCenter <= right {
			t.lastDir = ""
			t.sendCGI("/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
			t.lastBox = &box
			return
		}
		if (offset < 0 && boxCenter <= 0) || (offset > 0 && boxCenter >= 1) {
			t.lastDir = ""
			t.sendCGI("/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
			t.lastBox = &box
			return
		}
	}

	var dir string
	if offset < 0 {
		dir = "left"
	} else {
		dir = "right"
	}

	speedBucket := maxSpeed / 2
	stateKey := fmt.Sprintf("%s-%d", dir, speedBucket)
	if stateKey != t.lastDir {
		t.lastDir = stateKey
		t.sendCGI(fmt.Sprintf("/cgi-bin/ptzctrl.cgi?ptzcmd&%s&%d&%d", dir, maxSpeed, maxSpeed))
	}
	t.lastBox = &box
}

func (t *Tracker) handleTrackingLost() {
	if t.lastDir != "" {
		t.lastDir = ""
		t.sendCGI("/cgi-bin/ptzctrl.cgi?ptzcmd&ptzstop")
	}
	t.lastBox = nil

	// Execute the configured lost-target behavior from cameras.json.
	config := configCache.load(camerasFile)
	if config == nil {
		return
	}

	ip := GetCameraIP()
	if ip == "" {
		return
	}

	slot := findCameraSlot(config, ip)
	settings := getSettings(config, "trackingSettings", slot)
	if settings == nil {
		return
	}

	behavior, _ := settings["lostBehavior"].(string)
	switch behavior {
	case "preset":
		presetNum := 1
		if p, ok := settings["lostPreset"]; ok {
			switch v := p.(type) {
			case float64:
				presetNum = int(v)
			case string:
				fmt.Sscanf(v, "%d", &presetNum)
			}
		}
		t.sendCGI(fmt.Sprintf("/cgi-bin/ptzctrl.cgi?ptzcmd&poscall&%d", presetNum))
	default:
		// "stop" or anything else: PTZ stop already sent above.
	}
}

// sendCGI queues a PTZ command. If the queue is full the command is dropped
// (the next frame will re-evaluate and re-send if still needed).
func (t *Tracker) sendCGI(cgiPath string) {
	select {
	case t.cgiCh <- cgiPath:
	default:
	}
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
