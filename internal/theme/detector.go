package theme

import (
	"syscall"
	"unsafe"

	"gotracker/internal/winapi"
)

// Detector monitors Windows theme changes
type Detector struct {
	currentTheme Theme
	callback     func(Theme)
	stopChan     chan struct{}
}

// NewDetector creates a new theme detector
func NewDetector(callback func(Theme)) *Detector {
	return &Detector{
		callback: callback,
		stopChan: make(chan struct{}),
	}
}

// GetCurrentTheme reads the current theme from the registry
func GetCurrentTheme() Theme {
	// Open registry key: HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize
	var key syscall.Handle
	subKey := winapi.UTF16PtrFromString("Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize")

	err := winapi.RegOpenKeyEx(
		syscall.Handle(winapi.HKEY_CURRENT_USER),
		subKey,
		0,
		winapi.KEY_READ,
		&key,
	)
	if err != nil {
		// Default to light theme if we can't read the registry
		return Light
	}
	defer winapi.RegCloseKey(key)

	// Read AppsUseLightTheme value
	valueName := winapi.UTF16PtrFromString("AppsUseLightTheme")
	var valueType uint32
	var data [4]byte
	dataSize := uint32(4)

	err = winapi.RegQueryValueEx(
		key,
		valueName,
		nil,
		&valueType,
		&data[0],
		&dataSize,
	)
	if err != nil {
		// Default to light theme if value doesn't exist
		return Light
	}

	// Convert DWORD to uint32
	value := *(*uint32)(unsafe.Pointer(&data[0]))

	// 0 = Dark mode, 1 = Light mode
	if value == 0 {
		return Dark
	}
	return Light
}

// Start begins monitoring theme changes
func (d *Detector) Start() {
	d.currentTheme = GetCurrentTheme()

	// Start monitoring goroutine
	go d.monitor()
}

// Stop stops monitoring theme changes
func (d *Detector) Stop() {
	close(d.stopChan)
}

// monitor watches for registry changes
func (d *Detector) monitor() {
	for {
		select {
		case <-d.stopChan:
			return
		default:
			// Open registry key
			var key syscall.Handle
			subKey := winapi.UTF16PtrFromString("Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize")

			err := winapi.RegOpenKeyEx(
				syscall.Handle(winapi.HKEY_CURRENT_USER),
				subKey,
				0,
				winapi.KEY_READ|winapi.KEY_NOTIFY,
				&key,
			)
			if err != nil {
				return
			}

			// Wait for registry change (blocking call)
			err = winapi.RegNotifyChangeKeyValue(
				key,
				false,
				winapi.REG_NOTIFY_CHANGE_LAST_SET,
				0,
				false,
			)

			winapi.RegCloseKey(key)

			if err != nil {
				return
			}

			// Registry changed, check if theme actually changed
			newTheme := GetCurrentTheme()
			if newTheme != d.currentTheme {
				d.currentTheme = newTheme
				if d.callback != nil {
					d.callback(newTheme)
				}
			}
		}
	}
}

// CurrentTheme returns the current theme
func (d *Detector) CurrentTheme() Theme {
	return d.currentTheme
}
