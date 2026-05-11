package winapi

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	// DLL libraries
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	uxtheme  = windows.NewLazySystemDLL("uxtheme.dll")

	// user32.dll functions
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procLoadCursorW         = user32.NewProc("LoadCursorW")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procInvalidateRect      = user32.NewProc("InvalidateRect")
	procSendMessageW        = user32.NewProc("SendMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procGetClientRect       = user32.NewProc("GetClientRect")
	procFillRect            = user32.NewProc("FillRect")
	procDrawTextW           = user32.NewProc("DrawTextW")
	procGetWindowTextW      = user32.NewProc("GetWindowTextW")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procSetWindowPos        = user32.NewProc("SetWindowPos")

	// kernel32.dll functions
	procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")

	// gdi32.dll functions
	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procSetBkColor       = gdi32.NewProc("SetBkColor")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procDeleteObject     = gdi32.NewProc("DeleteObject")

	// advapi32.dll functions
	procRegOpenKeyExW           = advapi32.NewProc("RegOpenKeyExW")
	procRegQueryValueExW        = advapi32.NewProc("RegQueryValueExW")
	procRegCloseKey             = advapi32.NewProc("RegCloseKey")
	procRegNotifyChangeKeyValue = advapi32.NewProc("RegNotifyChangeKeyValue")

	// dwmapi.dll functions
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")

	// uxtheme.dll functions
	procSetWindowTheme = uxtheme.NewProc("SetWindowTheme")
)

// GetModuleHandle returns a module handle
func GetModuleHandle(moduleName *uint16) (syscall.Handle, error) {
	ret, _, err := procGetModuleHandleW.Call(uintptr(unsafe.Pointer(moduleName)))
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

// LoadCursor loads a cursor resource
func LoadCursor(instance syscall.Handle, cursorName uintptr) (syscall.Handle, error) {
	ret, _, err := procLoadCursorW.Call(uintptr(instance), cursorName)
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

// LoadImage loads an icon, cursor, or bitmap from a resource or file
func LoadImage(instance syscall.Handle, name uintptr, imageType uint32, cx, cy int, fuLoad uint32) (syscall.Handle, error) {
	ret, _, err := procLoadImageW.Call(uintptr(instance), name, uintptr(imageType), uintptr(cx), uintptr(cy), uintptr(fuLoad))
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

// RegisterClassEx registers a window class
func RegisterClassEx(wndClass *WNDCLASSEX) (uint16, error) {
	ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(wndClass)))
	if ret == 0 {
		return 0, err
	}
	return uint16(ret), nil
}

// CreateWindowEx creates a window
func CreateWindowEx(
	exStyle uint32,
	className *uint16,
	windowName *uint16,
	style uint32,
	x, y, width, height int32,
	parent syscall.Handle,
	menu syscall.Handle,
	instance syscall.Handle,
	param uintptr,
) (syscall.Handle, error) {
	ret, _, err := procCreateWindowExW.Call(
		uintptr(exStyle),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		uintptr(style),
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		uintptr(parent),
		uintptr(menu),
		uintptr(instance),
		param,
	)
	if ret == 0 {
		return 0, err
	}
	return syscall.Handle(ret), nil
}

// DefWindowProc calls the default window procedure
func DefWindowProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	ret, _, _ := procDefWindowProcW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret
}

// GetMessage retrieves a message from the message queue
func GetMessage(msg *MSG, hwnd syscall.Handle, msgFilterMin, msgFilterMax uint32) (bool, error) {
	ret, _, err := procGetMessageW.Call(
		uintptr(unsafe.Pointer(msg)),
		uintptr(hwnd),
		uintptr(msgFilterMin),
		uintptr(msgFilterMax),
	)
	if int32(ret) == -1 {
		return false, err
	}
	return ret != 0, nil
}

// TranslateMessage translates virtual-key messages
func TranslateMessage(msg *MSG) {
	procTranslateMessage.Call(uintptr(unsafe.Pointer(msg)))
}

// DispatchMessage dispatches a message to a window procedure
func DispatchMessage(msg *MSG) {
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(msg)))
}

// PostQuitMessage posts a quit message
func PostQuitMessage(exitCode int32) {
	procPostQuitMessage.Call(uintptr(exitCode))
}

// ShowWindow shows or hides a window
func ShowWindow(hwnd syscall.Handle, cmdShow int32) bool {
	ret, _, _ := procShowWindow.Call(uintptr(hwnd), uintptr(cmdShow))
	return ret != 0
}

// UpdateWindow updates a window
func UpdateWindow(hwnd syscall.Handle) bool {
	ret, _, _ := procUpdateWindow.Call(uintptr(hwnd))
	return ret != 0
}

// InvalidateRect invalidates a rectangle in a window
func InvalidateRect(hwnd syscall.Handle, rect *RECT, erase bool) bool {
	eraseVal := uintptr(0)
	if erase {
		eraseVal = 1
	}
	ret, _, _ := procInvalidateRect.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(rect)),
		eraseVal,
	)
	return ret != 0
}

// GetClientRect gets the client area rectangle
func GetClientRect(hwnd syscall.Handle, rect *RECT) bool {
	ret, _, _ := procGetClientRect.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(rect)),
	)
	return ret != 0
}

// SetWindowPos changes the size, position, and Z order of a window
func SetWindowPos(hwnd syscall.Handle, hwndInsertAfter syscall.Handle, x, y, cx, cy int32, flags uint32) bool {
	ret, _, _ := procSetWindowPos.Call(
		uintptr(hwnd),
		uintptr(hwndInsertAfter),
		uintptr(x),
		uintptr(y),
		uintptr(cx),
		uintptr(cy),
		uintptr(flags),
	)
	return ret != 0
}

// FillRect fills a rectangle with a brush
func FillRect(hdc syscall.Handle, rect *RECT, brush syscall.Handle) bool {
	ret, _, _ := procFillRect.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(rect)),
		uintptr(brush),
	)
	return ret != 0
}

// DrawText draws formatted text in a rectangle
func DrawText(hdc syscall.Handle, text *uint16, count int32, rect *RECT, format uint32) int32 {
	ret, _, _ := procDrawTextW.Call(
		uintptr(hdc),
		uintptr(unsafe.Pointer(text)),
		uintptr(count),
		uintptr(unsafe.Pointer(rect)),
		uintptr(format),
	)
	return int32(ret)
}

// GetWindowText gets window text
func GetSystemMetrics(nIndex int32) int32 {
	ret, _, _ := procGetSystemMetrics.Call(uintptr(nIndex))
	return int32(ret)
}

func GetWindowText(hwnd syscall.Handle, str *uint16, maxCount int32) int32 {
	ret, _, _ := procGetWindowTextW.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(str)),
		uintptr(maxCount),
	)
	return int32(ret)
}

// SendMessage sends a message to a window
func SendMessage(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	ret, _, _ := procSendMessageW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret
}

// PostMessage posts a message to a window
func PostMessage(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) bool {
	ret, _, _ := procPostMessageW.Call(
		uintptr(hwnd),
		uintptr(msg),
		wParam,
		lParam,
	)
	return ret != 0
}

// CreateSolidBrush creates a solid brush
func CreateSolidBrush(color uint32) syscall.Handle {
	ret, _, _ := procCreateSolidBrush.Call(uintptr(color))
	return syscall.Handle(ret)
}

// SetTextColor sets the text color
func SetTextColor(hdc syscall.Handle, color uint32) uint32 {
	ret, _, _ := procSetTextColor.Call(uintptr(hdc), uintptr(color))
	return uint32(ret)
}

// SetBkColor sets the background color
func SetBkColor(hdc syscall.Handle, color uint32) uint32 {
	ret, _, _ := procSetBkColor.Call(uintptr(hdc), uintptr(color))
	return uint32(ret)
}

// SetBkMode sets the background mode
func SetBkMode(hdc syscall.Handle, mode int32) int32 {
	ret, _, _ := procSetBkMode.Call(uintptr(hdc), uintptr(mode))
	return int32(ret)
}

// DeleteObject deletes a GDI object
func DeleteObject(obj syscall.Handle) bool {
	ret, _, _ := procDeleteObject.Call(uintptr(obj))
	return ret != 0
}

// RegOpenKeyEx opens a registry key
func RegOpenKeyEx(key syscall.Handle, subKey *uint16, options uint32, samDesired uint32, result *syscall.Handle) error {
	ret, _, _ := procRegOpenKeyExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(subKey)),
		uintptr(options),
		uintptr(samDesired),
		uintptr(unsafe.Pointer(result)),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// RegQueryValueEx queries a registry value
func RegQueryValueEx(key syscall.Handle, valueName *uint16, reserved *uint32, valueType *uint32, data *byte, dataSize *uint32) error {
	ret, _, _ := procRegQueryValueExW.Call(
		uintptr(key),
		uintptr(unsafe.Pointer(valueName)),
		uintptr(unsafe.Pointer(reserved)),
		uintptr(unsafe.Pointer(valueType)),
		uintptr(unsafe.Pointer(data)),
		uintptr(unsafe.Pointer(dataSize)),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// RegCloseKey closes a registry key
func RegCloseKey(key syscall.Handle) error {
	ret, _, _ := procRegCloseKey.Call(uintptr(key))
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// RegNotifyChangeKeyValue notifies about registry key changes
func RegNotifyChangeKeyValue(key syscall.Handle, watchSubtree bool, notifyFilter uint32, event syscall.Handle, asynchronous bool) error {
	watchVal := uintptr(0)
	if watchSubtree {
		watchVal = 1
	}
	asyncVal := uintptr(0)
	if asynchronous {
		asyncVal = 1
	}
	ret, _, _ := procRegNotifyChangeKeyValue.Call(
		uintptr(key),
		watchVal,
		uintptr(notifyFilter),
		uintptr(event),
		asyncVal,
	)
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// UTF16PtrFromString converts a Go string to a UTF-16 pointer
func UTF16PtrFromString(s string) *uint16 {
	ptr, _ := syscall.UTF16PtrFromString(s)
	return ptr
}

// DwmSetWindowAttribute sets a window attribute for Desktop Window Manager
func DwmSetWindowAttribute(hwnd syscall.Handle, attribute uint32, attributeValue *int32, attributeSize uint32) error {
	ret, _, _ := procDwmSetWindowAttribute.Call(
		uintptr(hwnd),
		uintptr(attribute),
		uintptr(unsafe.Pointer(attributeValue)),
		uintptr(attributeSize),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// SetWindowTheme sets the visual theme for a window
func SetWindowTheme(hwnd syscall.Handle, subAppName *uint16, subIdList *uint16) error {
	ret, _, _ := procSetWindowTheme.Call(
		uintptr(hwnd),
		uintptr(unsafe.Pointer(subAppName)),
		uintptr(unsafe.Pointer(subIdList)),
	)
	if ret != 0 {
		return syscall.Errno(ret)
	}
	return nil
}

// IDC_ARROW cursor constant
const IDC_ARROW = 32512
