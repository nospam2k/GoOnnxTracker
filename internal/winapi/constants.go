package winapi

// Window Messages
const (
	WM_CREATE       = 0x0001
	WM_DESTROY      = 0x0002
	WM_SIZE         = 0x0005
	WM_COMMAND      = 0x0111
	WM_CTLCOLORBTN  = 0x0135
	WM_CTLCOLOREDIT = 0x0133
	WM_ERASEBKGND   = 0x0014
	WM_PAINT        = 0x000F
	WM_DRAWITEM     = 0x002B
	WM_USER         = 0x0400
)

// Custom message for theme change
const WM_THEMECHANGE = WM_USER + 1

// Window Styles
const (
	WS_OVERLAPPED       = 0x00000000
	WS_CAPTION          = 0x00C00000
	WS_SYSMENU          = 0x00080000
	WS_THICKFRAME       = 0x00040000
	WS_MINIMIZEBOX      = 0x00020000
	WS_MAXIMIZEBOX      = 0x00010000
	WS_OVERLAPPEDWINDOW = WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_THICKFRAME | WS_MINIMIZEBOX | WS_MAXIMIZEBOX
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_VSCROLL          = 0x00200000
)

// Button Styles
const (
	BS_PUSHBUTTON = 0x00000000
	BS_OWNERDRAW  = 0x0000000B
)

// Edit Control Styles
const (
	ES_MULTILINE    = 0x0004
	ES_AUTOVSCROLL  = 0x0040
	ES_AUTOHSCROLL  = 0x0080
	ES_READONLY     = 0x0800
)

// Show Window Commands
const (
	SW_SHOW          = 5
	SW_SHOWMINIMIZED = 2
	SW_SHOWNORMAL    = 1
)

// Registry Constants
const (
	HKEY_CURRENT_USER = 0x80000001
	KEY_READ          = 0x20019
	KEY_NOTIFY        = 0x0010
	REG_NOTIFY_CHANGE_LAST_SET = 0x00000004
	REG_DWORD         = 4
)

// GDI Constants
const (
	TRANSPARENT = 1
	OPAQUE      = 2
)

// System Metrics
const (
	SM_CXSCREEN = 0
	SM_CYSCREEN = 1
)

// Color Constants
const (
	COLOR_WINDOW    = 5
	COLOR_BTNFACE   = 15
)

// Control IDs
const (
	IDC_BUTTON1 = 1001
	IDC_BUTTON2 = 1002
	IDC_BUTTON3 = 1003
	IDC_EDIT    = 1004
)

// DWM Window Attributes
const (
	DWMWA_USE_IMMERSIVE_DARK_MODE = 20
)

// Icon / LoadImage constants
const (
	IMAGE_ICON     = 1
	LR_DEFAULTSIZE = 0x0040

	WM_SETICON  = 0x0080
	ICON_SMALL  = 0
	ICON_BIG    = 1
)

// Owner Draw States
const (
	ODS_SELECTED = 0x0001
	ODS_FOCUS    = 0x0010
)

// Owner Draw Actions
const (
	ODA_DRAWENTIRE = 0x0001
)

// SetWindowPos Flags
const (
	SWP_NOZORDER = 0x0004
)

// Draw Text Flags
const (
	DT_CENTER    = 0x00000001
	DT_VCENTER   = 0x00000004
	DT_SINGLELINE = 0x00000020
)
