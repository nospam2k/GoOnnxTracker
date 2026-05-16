package main

/*
#cgo LDFLAGS: -lonnxruntime
#cgo CPPFLAGS: -I/usr/include -I/usr/local/include
#include <onnxruntime_c_api.h>

extern void init_ort();
extern int create_session(const char* model_path);
extern int run_inference(float* input, int64_t* shape, int shape_len, float** output, int64_t* output_shape);
extern void cleanup_ort();
extern void LogMessage(char* msg);
*/
import "C"

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"sync"
	"unsafe"
)

//export LogMessage
func LogMessage(msg *C.char) {
	Log("[Inference] %s", C.GoString(msg))
}

var keypointNames = []string{
	"nose",
	"left_eye", "right_eye",
	"left_ear", "right_ear",
	"left_shoulder", "right_shoulder",
	"left_elbow", "right_elbow",
	"left_wrist", "right_wrist",
	"left_hip", "right_hip",
	"left_knee", "right_knee",
	"left_ankle", "right_ankle",
}

// faceKeypoints defines the indices of face-related keypoints:
// 0=nose, 1=left_eye, 2=right_eye, 3=left_ear, 4=right_ear
var faceKeypoints = map[int]bool{0: true, 1: true, 2: true, 3: true, 4: true}

type Keypoint struct {
	Name  string  `json:"name"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Score float64 `json:"score"`
}

type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type Detection struct {
	Keypoints []Keypoint `json:"keypoints"`
	Box       Box        `json:"box"`
	Score     float64    `json:"score"`
}

type PoseMessage struct {
	Cmd       string     `json:"cmd"`
	Keypoints []Keypoint `json:"keypoints,omitempty"`
	Box       *Box       `json:"box,omitempty"`
}

// inputPool reuses the float32 input buffer across frames (256*256*3 = 196608 floats).
var inputPool = sync.Pool{
	New: func() interface{} {
		s := make([]float32, 256*256*3)
		return &s
	},
}

// resizePool reuses the RGBA image used for resize output.
var resizePool = sync.Pool{
	New: func() interface{} {
		return image.NewRGBA(image.Rect(0, 0, 256, 256))
	},
}

const (
	maxSizeRatio     = 3.0 // area ratio beyond which a detection is treated as a different-distance interloper
	switchLockFrames = 15  // consecutive frames a candidate must hold before we commit to a target switch
	edgeMargin       = 0.15 // fraction of frame width/height considered "at the edge"
	edgeClearFrames  = 8   // consecutive edge frames before history is cleared
)

type Inference struct {
	lastBox       *Box
	lastDetection *Detection // last accepted detection; returned during lock so camera holds position
	lockCandidate *Detection // switch candidate under probation
	lockCount     int        // how many consecutive frames lockCandidate has held
	edgeFrames    int        // consecutive frames the detection has been at the frame edge
}

func (inf *Inference) init() error {
	modelPath := C.CString("./model.onnx")
	defer C.free(unsafe.Pointer(modelPath))

	if ret := C.create_session(modelPath); ret != 0 {
		return fmt.Errorf("failed to create ONNX session")
	}

	return nil
}

func (inf *Inference) Infer(jpegData []byte) (*Detection, error) {
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, fmt.Errorf("decode jpeg: %w", err)
	}

	// Reuse resize buffer from pool.
	dst := resizePool.Get().(*image.RGBA)
	resizeImageInto(img, dst)

	// Reuse float input buffer from pool.
	inputPtr := inputPool.Get().(*[]float32)
	inputData := *inputPtr
	imageToFloat32Into(dst, inputData)
	resizePool.Put(dst)

	shape := [4]C.int64_t{1, 256, 256, 3}

	var outputPtr *C.float
	var outputShape [2]C.int64_t

	ret := C.run_inference(
		(*C.float)(unsafe.Pointer(&inputData[0])),
		(*C.int64_t)(unsafe.Pointer(&shape[0])),
		C.int(4),
		&outputPtr,
		(*C.int64_t)(unsafe.Pointer(&outputShape[0])),
	)
	inputPool.Put(inputPtr)

	if ret != 0 {
		return nil, fmt.Errorf("inference failed")
	}

	if outputPtr == nil {
		inf.lastBox = nil
		return nil, nil
	}

	// Output shape is [1, 6, 56] — read directly from C memory, no copy.
	const numDets = 6
	const elementsPerDet = 56
	const totalElements = numDets * elementsPerDet

	outputData := unsafe.Slice((*float32)(unsafe.Pointer(outputPtr)), totalElements)

	detection := inf.pickBestDetection(outputData, numDets, elementsPerDet)

	if detection == nil {
		inf.lastBox = nil
		inf.lastDetection = nil
		inf.lockCandidate = nil
		inf.lockCount = 0
		return nil, nil
	}

	// A switch is needed when the candidate has no overlap with the last accepted box
	// OR is dramatically larger/smaller (interloper at different depth).
	needsLock := inf.lastBox != nil &&
		(boxOverlap(&detection.Box, inf.lastBox) == 0 || !sizeCompatible(&detection.Box, inf.lastBox))

	if needsLock {
		if inf.lockCandidate != nil && boxOverlap(&detection.Box, &inf.lockCandidate.Box) > 0 {
			inf.lockCount++
		} else {
			inf.lockCandidate = detection
			inf.lockCount = 1
		}
		if inf.lockCount < switchLockFrames {
			// Hold on last accepted detection while candidate is on probation.
			return inf.lastDetection, nil
		}
		// Candidate held long enough — commit to the switch.
		inf.lockCandidate = nil
		inf.lockCount = 0
	} else {
		inf.lockCandidate = nil
		inf.lockCount = 0
	}

	// If the detection stays near the frame edge for several frames, the subject
	// is likely walking out. Clear history so re-entry from a new position
	// doesn't trigger the lock against stale edge data.
	cx := detection.Box.X + detection.Box.W/2
	if cx < edgeMargin || cx > 1.0-edgeMargin {
		inf.edgeFrames++
		if inf.edgeFrames >= edgeClearFrames {
			inf.lastBox = nil
			inf.lastDetection = nil
			inf.lockCandidate = nil
			inf.lockCount = 0
			inf.edgeFrames = 0
			return detection, nil
		}
	} else {
		inf.edgeFrames = 0
	}

	inf.lastBox = &detection.Box
	inf.lastDetection = detection
	return detection, nil
}

// pickBestDetection operates directly on the C output slice — no intermediate
// [][]float32 allocation. Size-compatible detections (similar area to lastBox)
// are preferred over incompatible ones so an interloper at a different depth
// doesn't displace the tracked subject.
func (inf *Inference) pickBestDetection(data []float32, numDets, stride int) *Detection {
	var compatible, incompatible []Detection

	for i := 0; i < numDets; i++ {
		det := data[i*stride : i*stride+stride]

		maxConf := 0.0
		for j := 0; j < 5; j++ {
			if conf := float64(det[j*3+2]); conf > maxConf {
				maxConf = conf
			}
		}
		if maxConf < 0.3 {
			continue
		}

		d := parseDetection(det)
		if len(d.Keypoints) < 2 {
			continue
		}

		if inf.lastBox == nil || sizeCompatible(&d.Box, inf.lastBox) {
			compatible = append(compatible, d)
		} else {
			incompatible = append(incompatible, d)
		}
	}

	var lastCX, lastCY float64
	if inf.lastBox != nil {
		lastCX = inf.lastBox.X + inf.lastBox.W/2
		lastCY = inf.lastBox.Y + inf.lastBox.H/2
	}

	if best := pickBest(compatible, inf.lastBox, lastCX, lastCY); best != nil {
		return best
	}
	return pickBest(incompatible, inf.lastBox, lastCX, lastCY)
}

// pickBest selects from a slice of detections using: overlap > proximity > area.
func pickBest(dets []Detection, lastBox *Box, lastCX, lastCY float64) *Detection {
	if len(dets) == 0 {
		return nil
	}
	var best *Detection
	maxOverlap := 0.0
	minDist := -1.0
	var proximityFallback *Detection
	maxArea := 0.0
	var areaFallback *Detection

	for i := range dets {
		d := &dets[i]
		if area := d.Box.W * d.Box.H; area > maxArea {
			maxArea = area
			areaFallback = d
		}
		if lastBox != nil {
			if ov := boxOverlap(&d.Box, lastBox); ov > maxOverlap {
				maxOverlap = ov
				best = d
			}
			cx := d.Box.X + d.Box.W/2
			cy := d.Box.Y + d.Box.H/2
			dx, dy := cx-lastCX, cy-lastCY
			if dist := dx*dx + dy*dy; minDist < 0 || dist < minDist {
				minDist = dist
				proximityFallback = d
			}
		}
	}

	if best != nil {
		return best
	}
	if proximityFallback != nil {
		return proximityFallback
	}
	return areaFallback
}

// sizeCompatible returns true if box's area is within maxSizeRatio of last.
func sizeCompatible(box, last *Box) bool {
	lastArea := last.W * last.H
	if lastArea == 0 {
		return true
	}
	ratio := (box.W * box.H) / lastArea
	return ratio <= maxSizeRatio && ratio >= 1.0/maxSizeRatio
}

func parseDetection(data []float32) Detection {
	var kpts []Keypoint
	maxConf := 0.0

	for i := 0; i < 17; i++ {
		y := float64(data[i*3])
		x := float64(data[i*3+1])
		conf := float64(data[i*3+2])

		if conf > maxConf {
			maxConf = conf
		}
		if faceKeypoints[i] && conf >= 0.4 {
			kpts = append(kpts, Keypoint{
				Name:  keypointNames[i],
				X:     x,
				Y:     y,
				Score: conf,
			})
		}
	}

	return Detection{
		Keypoints: kpts,
		Box:       faceBox(kpts),
		Score:     maxConf,
	}
}

// faceBox computes a bounding box tightly around the provided face keypoints,
// with a small padding so the box doesn't clip the face edges.
func faceBox(kpts []Keypoint) Box {
	if len(kpts) == 0 {
		return Box{}
	}

	minX, minY := kpts[0].X, kpts[0].Y
	maxX, maxY := kpts[0].X, kpts[0].Y

	for _, kp := range kpts[1:] {
		if kp.X < minX {
			minX = kp.X
		}
		if kp.X > maxX {
			maxX = kp.X
		}
		if kp.Y < minY {
			minY = kp.Y
		}
		if kp.Y > maxY {
			maxY = kp.Y
		}
	}

	const pad = 0.05
	return Box{
		X: minX - pad,
		Y: minY - pad,
		W: (maxX - minX) + pad*2,
		H: (maxY - minY) + pad*2,
	}
}

func boxOverlap(box1, box2 *Box) float64 {
	interX := min(box1.X+box1.W, box2.X+box2.W) - max(box1.X, box2.X)
	interY := min(box1.Y+box1.H, box2.Y+box2.H) - max(box1.Y, box2.Y)
	if interX <= 0 || interY <= 0 {
		return 0
	}
	return interX * interY
}

// resizeImageInto writes a nearest-neighbour 256x256 resize of src into dst,
// reusing the existing dst allocation.
func resizeImageInto(src image.Image, dst *image.RGBA) {
	bounds := src.Bounds()
	srcW := bounds.Max.X - bounds.Min.X
	srcH := bounds.Max.Y - bounds.Min.Y
	const dstW, dstH = 256, 256

	for y := 0; y < dstH; y++ {
		srcY := bounds.Min.Y + y*srcH/dstH
		for x := 0; x < dstW; x++ {
			srcX := bounds.Min.X + x*srcW/dstW
			r, g, b, a := src.At(srcX, srcY).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
		}
	}
}

// imageToFloat32Into fills an existing float32 slice from an RGBA image,
// reading directly from the packed Pix buffer instead of going through At().
func imageToFloat32Into(img *image.RGBA, out []float32) {
	bounds := img.Bounds()
	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			off := img.PixOffset(x, y)
			out[idx] = float32(img.Pix[off])
			out[idx+1] = float32(img.Pix[off+1])
			out[idx+2] = float32(img.Pix[off+2])
			idx += 3
		}
	}
}

func (det *Detection) ToJSON() []byte {
	msg := PoseMessage{
		Cmd:       "pose",
		Keypoints: det.Keypoints,
		Box:       &det.Box,
	}
	data, _ := json.Marshal(msg)
	return data
}

// NotFoundJSON returns the pre-marshaled constant from tracker.go.
func NotFoundJSON() []byte {
	return notFoundMsg
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
