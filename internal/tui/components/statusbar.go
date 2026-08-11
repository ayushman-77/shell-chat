package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// StatusBar displays connection info and current context.
type StatusBar struct {
	username    string
	guildName   string
	channelName string
	memberCount int
	connected   bool
	width       int
}

// NewStatusBar creates a new status bar.
func NewStatusBar(username string) StatusBar {
	return StatusBar{
		username:  username,
		connected: false,
	}
}

// View renders the status bar.
func (s StatusBar) View() string {
	var parts []string

	// Connection status
	if s.connected {
		parts = append(parts, styles.StatusBarOnline.Render("● ONLINE"))
	} else {
		parts = append(parts, styles.ErrorStyle.Render("○ OFFLINE"))
	}

	parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan).Render("👤 "+s.username))

	if s.guildName != "" && s.width > 70 {
		parts = append(parts, lipgloss.NewStyle().Foreground(styles.Pink).Render("🏠 "+s.guildName))
	}

	if s.channelName != "" {
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(styles.TextBright).Render("#"+s.channelName))
	}

	if s.memberCount > 0 && s.width > 90 {
		parts = append(parts, fmt.Sprintf("%d online", s.memberCount))
	}

	left := " " + strings.Join(parts, " │ ")
	leftLen := lipgloss.Width(left)

	availForHelp := s.width - leftLen

	var rightStr string
	if availForHelp >= 68 {
		rightStr = "Tab: Navigate │ Enter: Send │ Esc: Input │ /help: Guide │ ^C: Quit"
	} else if availForHelp >= 52 {
		rightStr = "Tab: Navigate │ Enter: Send │ /help: Guide │ ^C: Quit"
	} else if availForHelp >= 38 {
		rightStr = "Tab: Navigate │ Enter: Send │ ^C: Quit"
	} else if availForHelp >= 24 {
		rightStr = "Enter: Send │ ^C: Quit"
	} else if availForHelp >= 10 {
		rightStr = "^C: Quit"
	}

	right := styles.HelpStyle.Render(rightStr)
	rightLen := lipgloss.Width(right)

	spacing := s.width - leftLen - rightLen
	if spacing < 0 {
		spacing = 0
	}

	bar := left + strings.Repeat(" ", spacing) + right

	return styles.StatusBarStyle.Width(s.width).Render(bar)
}

func (s StatusBar) SetSize(w int) StatusBar           { s.width = w; return s }
func (s StatusBar) SetUsername(name string) StatusBar { s.username = name; return s }
func (s StatusBar) SetGuild(name string) StatusBar    { s.guildName = name; return s }
func (s StatusBar) SetChannel(name string) StatusBar  { s.channelName = name; return s }
func (s StatusBar) SetMemberCount(n int) StatusBar    { s.memberCount = n; return s }
func (s StatusBar) SetConnected(c bool) StatusBar     { s.connected = c; return s }
