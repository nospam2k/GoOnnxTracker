package main

/*
#cgo LDFLAGS: -lonnxruntime
#cgo CPPFLAGS: -I/usr/include -I/usr/local/include
#include <onnxruntime_c_api.h>
#include <stdlib.h>
#include <stdio.h>
#include <wchar.h>
#include <string.h>

// Include the DirectML header if on Windows
#ifdef _WIN32
#include <dml_provider_factory.h>
#include <windows.h>
#endif

static OrtEnv* g_env = NULL;
static OrtSessionOptions* g_opts = NULL;
static OrtSession* g_session = NULL;
static OrtMemoryInfo* g_meminfo = NULL;
static const OrtApi* g_ort = NULL;

void init_ort() {
    if (g_ort == NULL) {
        g_ort = OrtGetApiBase()->GetApi(ORT_API_VERSION);
    }
}

int create_session(const char* model_path) {
    init_ort();
    if (g_ort->CreateEnv(ORT_LOGGING_LEVEL_WARNING, "onnx", &g_env) != NULL)
        return -1;
    if (g_ort->CreateSessionOptions(&g_opts) != NULL)
        return -1;

    int used_dml = 0;

#ifdef _WIN32
    // Attempt to enable DirectML explicitly
    const OrtDmlApi* dml_api = NULL;
    // Query the API for the DirectML provider
    OrtStatus* dml_status = g_ort->GetExecutionProviderApi("DirectML", ORT_API_VERSION, (const void**)&dml_api);
    
    if (dml_status == NULL && dml_api != NULL) {
        // Device ID 0 is usually the primary GPU
        if (dml_api->SessionOptionsAppendExecutionProvider_DML(g_opts, 0) == NULL) {
            used_dml = 1;
            printf("Execution Provider: DirectML (GPU)\n");
        }
    } else {
        if (dml_status != NULL) g_ort->ReleaseStatus(dml_status);
    }
#endif

    if (!used_dml) {
        printf("Execution Provider: CPU\n");
    }
    fflush(stdout);

#ifdef _WIN32
    // Convert char* to wchar_t* on Windows for CreateSession
    int len = MultiByteToWideChar(CP_UTF8, 0, model_path, -1, NULL, 0);
    wchar_t* wide_path = (wchar_t*)malloc(len * sizeof(wchar_t));
    MultiByteToWideChar(CP_UTF8, 0, model_path, -1, wide_path, len);
    OrtStatus* status = g_ort->CreateSession(g_env, wide_path, g_opts, &g_session);
    free(wide_path);
    if (status != NULL) {
        g_ort->ReleaseStatus(status);
        return -1;
    }
#else
    if (g_ort->CreateSession(g_env, model_path, g_opts, &g_session) != NULL)
        return -1;
#endif

    if (g_ort->CreateCpuMemoryInfo(OrtArenaAllocator, OrtMemTypeDefault, &g_meminfo) != NULL)
        return -1;
        
    printf("ONNX Runtime session created successfully\n");
    fflush(stdout);
    return 0;
}

int run_inference(float* input, int64_t* shape, int shape_len, float** output, int64_t* output_shape) {
    init_ort();

    if (g_session == NULL)
        return -1;

    // Convert float input to int32 (model expects int32 0-255 values)
    int32_t* int_input = malloc(1*256*256*3*sizeof(int32_t));
    for (int i = 0; i < 1*256*256*3; i++) {
        int_input[i] = (int32_t)input[i];
    }

    OrtValue* input_tensor = NULL;
    OrtStatus* status = g_ort->CreateTensorWithDataAsOrtValue(g_meminfo, int_input, 1*256*256*3*sizeof(int32_t),
                                             shape, shape_len, ONNX_TENSOR_ELEMENT_DATA_TYPE_INT32,
                                             &input_tensor);
    if (status != NULL) {
        g_ort->ReleaseStatus(status);
        free(int_input);
        return -1;
    }

    const char* input_names[] = {"input:0"};
    const char* output_names[] = {"Identity:0"};
    OrtValue* output_tensor = NULL;

    status = g_ort->Run(g_session, NULL, input_names, (const OrtValue* const*)&input_tensor, 1,
                   output_names, 1, &output_tensor);
    if (status != NULL) {
        g_ort->ReleaseValue(input_tensor);
        g_ort->ReleaseStatus(status);
        free(int_input);
        return -1;
    }

    float* out_data = NULL;
    status = g_ort->GetTensorMutableData(output_tensor, (void**)&out_data);
    if (status != NULL) {
        g_ort->ReleaseValue(input_tensor);
        g_ort->ReleaseValue(output_tensor);
        g_ort->ReleaseStatus(status);
        free(int_input);
        return -1;
    }

    *output = out_data;
    free(int_input);
    return 0;
}

void cleanup_ort() {
    if (g_ort == NULL) return;
    if (g_session) g_ort->ReleaseSession(g_session);
    if (g_meminfo) g_ort->ReleaseMemoryInfo(g_meminfo);
    if (g_opts) g_ort->ReleaseSessionOptions(g_opts);
    if (g_env) g_ort->ReleaseEnv(g_env);
}
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