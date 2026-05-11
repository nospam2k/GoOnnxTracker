package theme

// ColorScheme represents colors for a theme
type ColorScheme struct {
	WindowBg uint32
	TextFg   uint32
	ButtonBg uint32
	EditBg   uint32
}

// Windows uses BGR format (0x00BBGGRR)
var (
	// Dark mode colors
	DarkScheme = ColorScheme{
		WindowBg: 0x00202020, // #202020
		TextFg:   0x00FFFFFF, // #FFFFFF
		ButtonBg: 0x002D2D2D, // #2D2D2D
		EditBg:   0x001E1E1E, // #1E1E1E
	}

	// Light mode colors
	LightScheme = ColorScheme{
		WindowBg: 0x00F0F0F0, // #F0F0F0
		TextFg:   0x00000000, // #000000
		ButtonBg: 0x00E1E1E1, // #E1E1E1
		EditBg:   0x00FFFFFF, // #FFFFFF
	}
)

// Theme type
type Theme int

const (
	Light Theme = iota
	Dark
)

// GetColorScheme returns the color scheme for a theme
func GetColorScheme(t Theme) ColorScheme {
	if t == Dark {
		return DarkScheme
	}
	return LightScheme
}
