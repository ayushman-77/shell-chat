package styles

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Force 256-color ANSI output so colors render richly even over headless SSH
	lipgloss.SetColorProfile(termenv.ANSI256)
}

// Color palette — High-contrast Vibrant ANSI 256 Cyber theme
var (
	// Base background & surfaces
	Background   = lipgloss.Color("234") // #1e1e2e
	Surface      = lipgloss.Color("236") // #313244
	SurfaceLight = lipgloss.Color("238") // #45475a
	SurfaceHover = lipgloss.Color("240") // #585b70
	Overlay      = lipgloss.Color("242")

	// Text levels
	Text       = lipgloss.Color("254") // #e2e8f0
	TextBright = lipgloss.Color("231") // Pure White
	TextDim    = lipgloss.Color("246") // Muted Slate
	TextMuted  = lipgloss.Color("243") // Dim Slate

	// Accents & State
	Primary     = lipgloss.Color("39")  // Electric Sky Blue
	PrimaryDark = lipgloss.Color("32")  // Deep Sapphire
	Secondary   = lipgloss.Color("46")  // Neon Emerald Green
	Accent      = lipgloss.Color("141") // Vibrant Lavender Purple
	Pink        = lipgloss.Color("207") // Neon Pink
	Cyan        = lipgloss.Color("51")  // Cyber Cyan
	Yellow      = lipgloss.Color("220") // Golden Yellow
	Peach       = lipgloss.Color("208") // Coral Orange
	Red         = lipgloss.Color("196") // Vivid Red
	Success     = lipgloss.Color("46")  // Bright Green
	Warning     = lipgloss.Color("208")
	Error       = lipgloss.Color("196")

	// User name colors — distinct, vibrant ANSI 256 colors for chat authors.
	// All colors are curated with high luminance & saturation to guarantee readability on dark backgrounds.
	UserColors = []lipgloss.Color{
		lipgloss.Color("51"),  // Neon Cyan
		lipgloss.Color("46"),  // Bright Emerald
		lipgloss.Color("141"), // Bright Lavender
		lipgloss.Color("220"), // Bright Gold
		lipgloss.Color("207"), // Hot Pink
		lipgloss.Color("208"), // Vivid Orange
		lipgloss.Color("39"),  // Sky Blue
		lipgloss.Color("49"),  // Mint Green
		lipgloss.Color("213"), // Orchid Rose
		lipgloss.Color("117"), // Light Periwinkle
		lipgloss.Color("118"), // Vivid Lime
		lipgloss.Color("214"), // Warm Amber
		lipgloss.Color("209"), // Bright Coral
		lipgloss.Color("205"), // Deep Flamingo
		lipgloss.Color("177"), // Neon Violet
		lipgloss.Color("50"),  // Electric Teal
		lipgloss.Color("221"), // Bright Gold
		lipgloss.Color("215"), // Vivid Apricot
		lipgloss.Color("154"), // Bright Chartreuse
		lipgloss.Color("75"),  // Soft Ice Blue
		lipgloss.Color("86"),  // Aquamarine
		lipgloss.Color("210"), // Vivid Salmon
		lipgloss.Color("135"), // Bright Purple
		lipgloss.Color("176"), // Light Orchid
		lipgloss.Color("183"), // Pastel Purple
		lipgloss.Color("147"), // Periwinkle
		lipgloss.Color("121"), // Bright Seafoam
		lipgloss.Color("216"), // Radiant Peach
		lipgloss.Color("201"), // Bright Magenta
		lipgloss.Color("44"),  // Turquoise Cyan
		lipgloss.Color("218"), // Soft Rose
		lipgloss.Color("48"),  // Neon Spring Green
	}
)

// Pre-built styles for consistent UI rendering.
var (
	SidebarStyle = lipgloss.NewStyle().
			Foreground(Text)

	SidebarHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(Cyan).
			Padding(0, 1)

	ServerBadge = lipgloss.NewStyle().
			Background(Accent).
			Foreground(Background).
			Bold(true).
			Padding(0, 1)

	ChannelActive = lipgloss.NewStyle().
			Foreground(TextBright).
			Background(PrimaryDark).
			Bold(true).
			Padding(0, 1)

	ChannelInactive = lipgloss.NewStyle().
			Foreground(TextDim).
			Padding(0, 1)

	GuildName = lipgloss.NewStyle().
			Foreground(Pink).
			Bold(true).
			Padding(0, 1)

	MessageAuthor = lipgloss.NewStyle().
			Bold(true)

	MessageTime = lipgloss.NewStyle().
			Foreground(TextMuted)

	MessageContent = lipgloss.NewStyle().
			Foreground(TextBright)

	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(SurfaceHover).
			Padding(0, 1)

	InputFocused = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(0, 1)

	StatusBarStyle = lipgloss.NewStyle().
			Background(Surface).
			Foreground(Text)

	StatusBarOnline = lipgloss.NewStyle().
			Foreground(Success).
			Bold(true)

	ModalStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(Accent).
			Background(Surface).
			Padding(1, 2).
			Align(lipgloss.Center)

	TitleStyle = lipgloss.NewStyle().
			Foreground(Cyan).
			Bold(true)

	ErrorStyle = lipgloss.NewStyle().
			Foreground(Error).
			Bold(true)

	HelpStyle = lipgloss.NewStyle().
			Foreground(TextDim)
)

// UsernameColor returns a deterministic, distinct, and vibrant color encoded from a username.
func UsernameColor(username string) lipgloss.Color {
	trimmed := strings.ToLower(strings.TrimSpace(username))
	if trimmed == "" {
		return UserColors[0]
	}

	// 32-bit FNV-1a hash algorithm for uniform distribution across names
	var h uint32 = 2166136261
	for i := 0; i < len(trimmed); i++ {
		h ^= uint32(trimmed[i])
		h *= 16777619
	}

	idx := int(h % uint32(len(UserColors)))
	return UserColors[idx]
}

// UserColor returns a consistent color for a user based on their ID.
func UserColor(userID int64) lipgloss.Color {
	idx := int(userID % int64(len(UserColors)))
	if idx < 0 {
		idx = -idx
	}
	return UserColors[idx]
}

