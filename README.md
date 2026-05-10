# GoOnnxTracker

Real-time pose tracking with ONNX inference and PTZ camera control.

## Features

- Backend ONNX pose detection (17 keypoints)
- WebSocket real-time keypoint streaming to browser
- PTZ camera control (pan/tilt/zoom)
- Dead zone tracking with visual overlay
- Network camera discovery
- Preset management

## Quick Start

Go to [Releases](../../releases) and download the binary for your platform:

- **Windows**: `gotracker-windows.exe` + `onnxruntime.dll` (place in same directory)
- **macOS**: `gotracker-macos` (works on both Intel & Apple Silicon)
- **Linux**: `gotracker-linux` (x86_64)

Then run:

**Windows:**
```batch
gotracker-windows.exe --addr :4000
```

**macOS/Linux:**
```bash
chmod +x gotracker-macos
./gotracker-macos --addr :4000
```

Open http://localhost:4000 in your browser.

## Releases & Builds

Binaries are automatically built for **Windows, macOS, and Linux** via GitHub Actions whenever a version tag is pushed (e.g., `git tag v1.0.0 && git push origin v1.0.0`).

Check [Releases](../../releases) for the latest pre-built binaries.

## Usage

1. Open `http://localhost:4000` in browser
2. Add a camera (by IP address)
3. Select camera in viewport
4. Enable auto-tracking toggle
5. Adjust dead zone and tracking speed

## Architecture

- `main.go`: HTTP server, WebSocket hub, API endpoints
- `inference.go`: ONNX inference via C API (cgo)
- `tracker.go`: Frame polling and detection loop
- `hub.go`: WebSocket connection management
- `static/index.html`: Browser UI with tracking overlay

## Supported Platforms

- ✅ Windows (x86_64)
- ✅ macOS (Intel & Apple Silicon)
- ✅ Linux (x86_64)

All with native ONNX Runtime support compiled in!
