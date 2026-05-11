package ui

import (
	"syscall"

	"gotracker/internal/winapi"
)

// CreateButton creates a button control
func CreateButton(parent syscall.Handle, instance syscall.Handle, text string, x, y, width, height int32, id int) (syscall.Handle, error) {
	return winapi.CreateWindowEx(
		0,
		winapi.UTF16PtrFromString("BUTTON"),
		winapi.UTF16PtrFromString(text),
		winapi.WS_CHILD|winapi.WS_VISIBLE|winapi.BS_OWNERDRAW,
		x, y, width, height,
		parent,
		syscall.Handle(id),
		instance,
		0,
	)
}

// CreateEdit creates an edit control
func CreateEdit(parent syscall.Handle, instance syscall.Handle, x, y, width, height int32, id int) (syscall.Handle, error) {
	return winapi.CreateWindowEx(
		0,
		winapi.UTF16PtrFromString("EDIT"),
		nil,
		winapi.WS_CHILD|winapi.WS_VISIBLE|winapi.WS_VSCROLL|winapi.ES_MULTILINE|winapi.ES_AUTOVSCROLL|winapi.ES_AUTOHSCROLL,
		x, y, width, height,
		parent,
		syscall.Handle(id),
		instance,
		0,
	)
}
