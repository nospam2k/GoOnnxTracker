#include <onnxruntime_c_api.h>
#include <stdlib.h>
#include <stdio.h>
#include <wchar.h>
#include <string.h>

#ifdef _WIN32
#include <windows.h>
#endif

extern void LogMessage(char* msg);

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

#ifdef _WIN32
    HMODULE dml_dll = LoadLibraryA("DirectML.dll");
    if (dml_dll != NULL) {
        LogMessage("GPU: DirectML available");
        FreeLibrary(dml_dll);
    } else {
        LogMessage("GPU: CPU only (DirectML not found)");
    }
#else
    LogMessage("GPU: CPU only (non-Windows platform)");
#endif

#ifdef _WIN32
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

    LogMessage("ONNX Runtime session created successfully");
    return 0;
}

int run_inference(float* input, int64_t* shape, int shape_len, float** output, int64_t* output_shape) {
    init_ort();

    if (g_session == NULL)
        return -1;

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
