package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var (
	addr = flag.String("addr", ":4000", "HTTP listen address")

	mu         sync.RWMutex
	cameraURL  string
	hub        *Hub
	inference  *Inference
	tracker    *Tracker
	trackingCh chan []byte

	configDir   = "./config"
	camerasFile = filepath.Join(configDir, "cameras.json")
)

// ── Web server lifecycle ───────────────────────────────────────

var (
	serverMu      sync.Mutex
	serverRunning bool
	serverCancel  context.CancelFunc
	serverDone    chan struct{}
	logWriter     io.Writer
)

func IsWebServerRunning() bool {
	serverMu.Lock()
	defer serverMu.Unlock()
	return serverRunning
}

func StartWebServer(logChan chan string) error {
	serverMu.Lock()
	if serverRunning {
		serverMu.Unlock()
		return fmt.Errorf("server already running")
	}

	ctx, cancel := context.WithCancel(context.Background())
	serverCancel = cancel
	serverRunning = true
	serverDone = make(chan struct{})
	serverMu.Unlock()

	// Set up log writer to send to logChan if provided
	if logChan != nil {
		logWriter = &logChannelWriter{ch: logChan}
		log.SetOutput(logWriter)
	}

	go func() {
		defer func() {
			serverMu.Lock()
			serverRunning = false
			close(serverDone)
			serverMu.Unlock()
		}()

		// Hub goroutine
		go hub.Run()

		// Tracker goroutine
		go tracker.Run()

		// HTTP server
		mux := http.NewServeMux()
		mux.HandleFunc("/ws", handleWS)
		mux.HandleFunc("/stream", handleStream)
		mux.HandleFunc("/config", handleConfig)
		mux.HandleFunc("/api/cameras", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				handlePostCameras(w, r)
			} else {
				handleGetCameras(w, r)
			}
		})
		mux.HandleFunc("/api/snapshot/", handleSnapshot)
		mux.HandleFunc("/api/cgi/", handleCGI)
		mux.HandleFunc("/api/visca/", handleVISCANamed)
		mux.HandleFunc("/api/visca-raw/", handleVISCARaw)
		mux.HandleFunc("/api/visca", handleVISCA)
		mux.HandleFunc("/api/local-subnet", handleLocalSubnet)
		mux.HandleFunc("/api/scan", handleScan)
		mux.Handle("/", http.FileServer(http.Dir("./static")))

		srv := &http.Server{
			Addr:    *addr,
			Handler: mux,
		}

		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			srv.Shutdown(shutdownCtx)
		}()

		Log("OnnxTracker running at http://localhost%s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			Log("Server error: %v", err)
		}
	}()

	return nil
}

func StopWebServer() {
	serverMu.Lock()
	if !serverRunning {
		serverMu.Unlock()
		return
	}
	cancel := serverCancel
	serverMu.Unlock()

	cancel()
	<-serverDone
}

type logChannelWriter struct {
	ch chan string
}

func (w *logChannelWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimRight(string(p), "\n")
	if msg != "" {
		select {
		case w.ch <- msg:
		default:
			// Channel full, drop message
		}
	}
	return len(p), nil
}

func serverMain() {
	flag.Parse()

	if inference == nil {
		inference = &Inference{}
		if err := inference.init(); err != nil {
			log.Fatalf("Failed to initialize inference: %v", err)
		}
	}

	if hub == nil {
		hub = NewHub()
	}

	if tracker == nil {
		trackingCh = make(chan []byte, 8)
		tracker = &Tracker{
			inference: inference,
			hub:       hub,
			frameCh:   trackingCh,
		}
	}

	if err := StartWebServer(nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	select {}
}

// ── HTTP handlers ───────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleGetCameras(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(camerasFile)
	if os.IsNotExist(err) {
		writeJSON(w, 200, map[string]any{})
		return
	}
	if err != nil {
		Log("[cameras] load error: %v", err)
		writeJSON(w, 200, map[string]any{})
		return
	}
	var cameras any
	if err := json.Unmarshal(data, &cameras); err != nil {
		Log("[cameras] parse error: %v", err)
		writeJSON(w, 200, map[string]any{})
		return
	}
	writeJSON(w, 200, cameras)
}

func handlePostCameras(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false})
		return
	}
	if err := os.MkdirAll(configDir, 0755); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false})
		return
	}
	var pretty strings.Builder
	var raw any
	json.Unmarshal(body, &raw)
	enc := json.NewEncoder(&pretty)
	enc.SetIndent("", "  ")
	enc.Encode(raw)
	if err := os.WriteFile(camerasFile, []byte(pretty.String()), 0644); err != nil {
		Log("[cameras] save error: %v", err)
		writeJSON(w, 500, map[string]any{"ok": false})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/snapshot/")
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + ip + "/snapshot.jpg")
	if err != nil {
		w.WriteHeader(502)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	io.Copy(w, resp.Body)
}

func handleCGI(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/cgi/")
	cgiPath := r.URL.Query().Get("q")
	if cgiPath == "" {
		writeJSON(w, 400, map[string]any{"error": "missing q param"})
		return
	}
	target := "http://" + ip + cgiPath
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		w.WriteHeader(502)
		w.Write([]byte(err.Error()))
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "text/plain")
	io.Copy(w, resp.Body)
}

func viscaNibbles(pos uint16) [4]byte {
	return [4]byte{
		byte((pos >> 12) & 0xF),
		byte((pos >> 8) & 0xF),
		byte((pos >> 4) & 0xF),
		byte(pos & 0xF),
	}
}

func handleVISCA(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IP       string `json:"ip"`
		Type     string `json:"type"`
		Position uint16 `json:"position"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, 400, map[string]any{"error": "bad request"})
		return
	}
	var subcmd byte
	switch body.Type {
	case "zoom":
		subcmd = 0x47
	case "focus":
		subcmd = 0x48
	default:
		writeJSON(w, 400, map[string]any{"error": "unknown type"})
		return
	}
	n := viscaNibbles(body.Position)
	pkt := []byte{0x81, 0x01, 0x04, subcmd, n[0], n[1], n[2], n[3], 0xFF}

	conn, err := net.DialTimeout("tcp", body.IP+":5678", 2*time.Second)
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = conn.Write(pkt); err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	buf := make([]byte, 32)
	nr, err := conn.Read(buf)
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "VISCA timeout"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "resp": buf[:nr]})
}

const viscaUDPPort = 1259

var viscaCommands = map[string][]byte{
	"focus_auto":        {0x81, 0x01, 0x04, 0x38, 0x02, 0xFF},
	"focus_manual":      {0x81, 0x01, 0x04, 0x38, 0x03, 0xFF},
	"focus_auto_manual": {0x81, 0x01, 0x04, 0x38, 0x10, 0xFF},
}

func fetchCameraName(ip string) string {
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + ip + "/cgi-bin/param.cgi?get_device_conf")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	re := regexp.MustCompile(`devname=([^\r\n&]+)`)
	if m := re.FindSubmatch(body); m != nil {
		return strings.TrimSpace(string(m[1]))
	}
	return ""
}

func handleVISCANamed(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/visca/")
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeJSON(w, 400, map[string]any{"error": "missing name"})
		return
	}
	pkt, ok := viscaCommands[body.Name]
	if !ok {
		writeJSON(w, 400, map[string]any{"error": "unknown command: " + body.Name})
		return
	}

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", ip, viscaUDPPort))
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err = conn.Write(pkt); err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}

	buf := make([]byte, 32)
	if _, err = conn.Read(buf); err != nil {
		writeJSON(w, 502, map[string]any{"error": "VISCA timeout"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func handleVISCARaw(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/visca-raw/")
	var body struct {
		Cmd []byte `json:"cmd"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Cmd) == 0 {
		writeJSON(w, 400, map[string]any{"error": "missing cmd"})
		return
	}

	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", ip, viscaUDPPort))
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))

	if _, err = conn.Write(body.Cmd); err != nil {
		writeJSON(w, 502, map[string]any{"error": err.Error()})
		return
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		writeJSON(w, 502, map[string]any{"error": "VISCA timeout"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "resp": buf[:n]})
}

func getLocalSubnet() string {
	ip := getLocalIP()
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return strings.Join(parts[:3], ".")
	}
	return ""
}

func getLocalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}
			parts := strings.Split(ip.String(), ".")
			if len(parts) == 4 {
				return ip.String()
			}
		}
	}
	return ""
}

func handleLocalSubnet(w http.ResponseWriter, r *http.Request) {
	subnet := getLocalSubnet()
	if subnet == "" {
		writeJSON(w, 200, map[string]any{"subnet": nil})
	} else {
		writeJSON(w, 200, map[string]any{"subnet": subnet})
	}
}

func probePort(ip string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ip, port), timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func handleScan(w http.ResponseWriter, r *http.Request) {
	subnet := getLocalSubnet()
	if subnet == "" {
		writeJSON(w, 200, map[string]any{"cameras": []any{}})
		return
	}

	type result struct {
		IP   string `json:"ip"`
		Name string `json:"name,omitempty"`
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	var cameras []result

	for i := 1; i <= 254; i++ {
		ip := fmt.Sprintf("%s.%d", subnet, i)
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			if !probePort(ip, 5678, 500*time.Millisecond) {
				return
			}
			name := fetchCameraName(ip)
			mu.Lock()
			cameras = append(cameras, result{IP: ip, Name: name})
			mu.Unlock()
		}(ip)
	}

	wg.Wait()
	if cameras == nil {
		cameras = []result{}
	}
	writeJSON(w, 200, map[string]any{"cameras": cameras})
}

func handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r, nil, 1024, 1024)
	if err != nil {
		Log("WebSocket upgrade error: %v", err)
		return
	}
	client := &Client{Hub: hub, Conn: conn, Send: make(chan []byte, 256)}
	hub.Register <- client
	client.Start()
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	mu.RLock()
	url := cameraURL
	mu.RUnlock()

	if url == "" {
		http.Error(w, "no camera configured", http.StatusServiceUnavailable)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "camera unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	contentType := resp.Header.Get("Content-Type")

	if strings.Contains(contentType, "multipart") {
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		io.Copy(w, resp.Body)
	} else {
		streamSnapshots(w, r, url)
	}
}

func streamSnapshots(w http.ResponseWriter, r *http.Request, url string) {
	boundary := "snapshot-boundary"
	w.Header().Set("Content-Type", fmt.Sprintf("multipart/x-mixed-replace; boundary=%s", boundary))
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return
			}

			var buf bytes.Buffer
			io.Copy(&buf, resp.Body)
			resp.Body.Close()
			frameData := buf.Bytes()

			fmt.Fprintf(w, "--%s\r\n", boundary)
			fmt.Fprintf(w, "Content-Type: image/jpeg\r\n")
			fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(frameData))
			w.Write(frameData)
			fmt.Fprintf(w, "\r\n")
			flusher.Flush()
		}
	}
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg struct {
		CameraURL string `json:"cameraUrl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	mu.Lock()
	cameraURL = cfg.CameraURL
	mu.Unlock()

	w.WriteHeader(http.StatusOK)
}
