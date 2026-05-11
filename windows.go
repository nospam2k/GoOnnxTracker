//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"

	"gotracker/internal/theme"
	"gotracker/internal/ui"
	"gotracker/internal/winapi"
)

type WindowsGUI struct {
	mainWindow    syscall.Handle
	editControl   syscall.Handle
	hInstance     syscall.Handle
	currentTheme  theme.Theme
	currentColors theme.ColorScheme
	windowBrush   syscall.Handle
	editBrush     syscall.Handle
	detector      *theme.Detector
}

func NewGUI() GUI { return &WindowsGUI{} }

func (g *WindowsGUI) Run(logChan chan string) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	g.hInstance, _ = winapi.GetModuleHandle(nil)
	cursor, _ := winapi.LoadCursor(0, winapi.IDC_ARROW)

	g.currentTheme = theme.GetCurrentTheme()
	g.currentColors = theme.GetColorScheme(g.currentTheme)
	g.windowBrush = winapi.CreateSolidBrush(g.currentColors.WindowBg)
	g.editBrush = winapi.CreateSolidBrush(g.currentColors.EditBg)

	className := winapi.UTF16PtrFromString("OnnxTrackerClass")
	wndClass := winapi.WNDCLASSEX{
		Size:       uint32(unsafe.Sizeof(winapi.WNDCLASSEX{})),
		WndProc:    syscall.NewCallback(g.wndProc),
		Instance:   g.hInstance,
		Cursor:     cursor,
		Background: g.windowBrush,
		ClassName:  className,
	}
	winapi.RegisterClassEx(&wndClass)

	g.mainWindow, _ = winapi.CreateWindowEx(
		0, className, winapi.UTF16PtrFromString("OnnxTracker Control Panel"),
		winapi.WS_OVERLAPPEDWINDOW, 100, 100, 500, 300, 0, 0, g.hInstance, 0,
	)

	useDarkMode := int32(0)
	if g.currentTheme == theme.Dark {
		useDarkMode = 1
	}
	winapi.DwmSetWindowAttribute(g.mainWindow, winapi.DWMWA_USE_IMMERSIVE_DARK_MODE, &useDarkMode, 4)

	// Create edit control for logs
	g.editControl, _ = ui.CreateEdit(g.mainWindow, g.hInstance, 10, 10, 0, 0, winapi.IDC_EDIT)

	if g.currentTheme == theme.Dark {
		winapi.SetWindowTheme(g.editControl, winapi.UTF16PtrFromString("DarkMode_Explorer"), nil)
	} else {
		winapi.SetWindowTheme(g.editControl, winapi.UTF16PtrFromString("Explorer"), nil)
	}

	winapi.ShowWindow(g.mainWindow, winapi.SW_SHOWNORMAL)
	winapi.UpdateWindow(g.mainWindow)
	winapi.ShowWindow(g.mainWindow, winapi.SW_SHOWMINIMIZED)
	winapi.UpdateWindow(g.mainWindow)
	g.layoutControls(g.mainWindow)

	go func() {
		for msg := range logChan {
			g.AppendLog(msg + "\r\n")
		}
	}()

	var msg winapi.MSG
	for {
		hasMsg, _ := winapi.GetMessage(&msg, 0, 0, 0)
		if !hasMsg {
			break
		}
		winapi.TranslateMessage(&msg)
		winapi.DispatchMessage(&msg)
	}
	return nil
}

func (g *WindowsGUI) AppendLog(text string) {
	textLen := winapi.SendMessage(g.editControl, 0x000E, 0, 0)
	winapi.SendMessage(g.editControl, 0x00B1, textLen, textLen)
	winapi.SendMessage(g.editControl, 0x00C2, 0, uintptr(unsafe.Pointer(winapi.UTF16PtrFromString(text))))
}

func (g *WindowsGUI) SetButtonText(button int, text string) {
	// No-op: log-only window has no button
}

func (g *WindowsGUI) wndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case winapi.WM_CTLCOLOREDIT:
		hdc := syscall.Handle(wParam)
		winapi.SetTextColor(hdc, 0x00AAAAAA)
		winapi.SetBkColor(hdc, g.currentColors.EditBg)
		return uintptr(g.editBrush)

	case winapi.WM_ERASEBKGND:
		hdc := syscall.Handle(wParam)
		var rect winapi.RECT
		winapi.GetClientRect(hwnd, &rect)
		winapi.FillRect(hdc, &rect, g.windowBrush)
		return 1

	case winapi.WM_SIZE:
		g.layoutControls(hwnd)

	case winapi.WM_DESTROY:
		winapi.PostQuitMessage(0)
	}
	return winapi.DefWindowProc(hwnd, msg, wParam, lParam)
}

// layoutControls repositions the edit control to fill the client area.
func (g *WindowsGUI) layoutControls(hwnd syscall.Handle) {
	var rect winapi.RECT
	winapi.GetClientRect(hwnd, &rect)
	w := int32(rect.Right - rect.Left)
	h := int32(rect.Bottom - rect.Top)

	const margin = 10

	winapi.SetWindowPos(g.editControl, 0, margin, margin, w-margin*2, h-margin*2, winapi.SWP_NOZORDER)
}

func (g *WindowsGUI) Cleanup() {
	winapi.DeleteObject(g.windowBrush)
	winapi.DeleteObject(g.editBrush)
}
