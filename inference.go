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
	Cmd       string      `json:"cmd"`
	Keypoints []Keypoint  `json:"keypoints,omitempty"`
	Box       *Box        `json:"box,omitempty"`
}

type Inference struct {
	lastBox *Box
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

	resized := resizeImage(img, 256, 256)
	inputData := imageToFloat32(resized)

	// Prepare input tensor shape: (1, 256, 256, 3)
	shape := [4]C.int64_t{1, 256, 256, 3}

	var outputPtr *C.float
	var outputShape [2]C.int64_t

	// Run inference
	if ret := C.run_inference(
		(*C.float)(unsafe.Pointer(&inputData[0])),
		(*C.int64_t)(unsafe.Pointer(&shape[0])),
		C.int(4),
		&outputPtr,
		(*C.int64_t)(unsafe.Pointer(&outputShape[0])),
	); ret != 0 {
		return nil, fmt.Errorf("inference failed")
	}

	if outputPtr == nil {
		inf.lastBox = nil
		return nil, nil
	}

	// Output shape is [1, 6, 56] - batch=1, 6 detections, 56 values per detection
	const numDets = 6
	const elementsPerDet = 56
	totalElements := numDets * elementsPerDet

	// Extract output data
	outputSlice := unsafe.Slice(outputPtr, totalElements)
	var outputData []float32
	for i := 0; i < len(outputSlice); i++ {
		outputData = append(outputData, float32(outputSlice[i]))
	}

	if len(outputData) < elementsPerDet {
		inf.lastBox = nil
		return nil, nil
	}

	// Parse detections
	var detections [][]float32
	for i := 0; i < numDets; i++ {
		if i*elementsPerDet+elementsPerDet <= len(outputData) {
			det := make([]float32, elementsPerDet)
			copy(det, outputData[i*elementsPerDet:(i+1)*elementsPerDet])
			detections = append(detections, det)
		}
	}

	detection := inf.pickBestDetection(detections)
	if detection != nil {
		inf.lastBox = &detection.Box
	} else {
		inf.lastBox = nil
	}
	return detection, nil
}

func (inf *Inference) pickBestDetection(detections [][]float32) *Detection {
	var best *Detection
	maxOverlap := 0.0
	maxArea := 0.0
	var areaFallback *Detection

	for _, det := range detections {
		if len(det) < 56 {
			continue
		}

		maxConf := 0.0
		for i := 0; i < 17; i++ {
			conf := float64(det[i*3+2])
			if conf > maxConf {
				maxConf = conf
			}
		}

		if maxConf < 0.3 {
			continue
		}

		d := parseDetection(det)

		// Require at least 4 confident keypoints to count as a real person
		if len(d.Keypoints) < 4 {
			continue
		}

		// Always track the largest-area candidate as a fallback
		area := d.Box.W * d.Box.H
		if area > maxArea {
			maxArea = area
			areaFallback = &d
		}

		if inf.lastBox != nil {
			overlap := boxOverlap(&d.Box, inf.lastBox)
			if overlap > maxOverlap {
				maxOverlap = overlap
				best = &d
			}
		}
	}

	// If overlap search found nothing (camera panned, subject shifted),
	// fall back to largest detection to re-acquire rather than going lost.
	if best == nil {
		best = areaFallback
	}

	return best
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
		if conf >= 0.4 {
			kpts = append(kpts, Keypoint{
				Name:  keypointNames[i],
				X:     x,
				Y:     y,
				Score: conf,
			})
		}
	}

	var boxVals [4]float64
	if len(data) >= 55 {
		boxVals[0] = float64(data[51])
		boxVals[1] = float64(data[52])
		boxVals[2] = float64(data[53])
		boxVals[3] = float64(data[54])
	}

	box := Box{
		X: boxVals[1],
		Y: boxVals[0],
		W: boxVals[3] - boxVals[1],
		H: boxVals[2] - boxVals[0],
	}

	return Detection{
		Keypoints: kpts,
		Box:       box,
		Score:     maxConf,
	}
}

func boxOverlap(box1, box2 *Box) float64 {
	x1Left := box1.X
	x1Right := box1.X + box1.W
	y1Top := box1.Y
	y1Bottom := box1.Y + box1.H

	x2Left := box2.X
	x2Right := box2.X + box2.W
	y2Top := box2.Y
	y2Bottom := box2.Y + box2.H

	interX := min(x1Right, x2Right) - max(x1Left, x2Left)
	interY := min(y1Bottom, y2Bottom) - max(y1Top, y2Top)
	if interX < 0 || interY < 0 {
		return 0
	}
	return interX * interY
}

func resizeImage(src image.Image, width, height int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Max.X - bounds.Min.X
	srcHeight := bounds.Max.Y - bounds.Min.Y

	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := x * srcWidth / width
			srcY := y * srcHeight / height
			r, g, b, a := src.At(srcX, srcY).RGBA()
			dst.SetRGBA(x, y, color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)})
		}
	}
	return dst
}

func imageToFloat32(img image.Image) []float32 {
	bounds := img.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y

	data := make([]float32, w*h*3)
	idx := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			data[idx] = float32(r >> 8)
			data[idx+1] = float32(g >> 8)
			data[idx+2] = float32(b >> 8)
			idx += 3
		}
	}
	return data
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

func NotFoundJSON() []byte {
	msg := PoseMessage{Cmd: "notfound"}
	data, _ := json.Marshal(msg)
	return data
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