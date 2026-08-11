package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// ModalType identifies the kind of modal.
type ModalType int

const (
	ModalCreateGuild ModalType = iota
	ModalCreateChannel
	ModalHelp
	ModalConfirm
	ModalSettings
)

// CreateGuildMsg is emitted when a guild is created via modal.
type CreateGuildMsg struct{ Name string }

// CreateChannelMsg is emitted when a channel is created via modal.
type CreateChannelMsg struct{ Name string }

// SaveSettingsMsg is emitted when a user saves updated settings.
type SaveSettingsMsg struct {
	NewUsername     string
	CurrentPassword string
	NewPassword     string
}

// Modal is a dialog overlay.
type Modal struct {
	modalType     ModalType
	title         string
	input         textinput.Model
	currPassInput textinput.Model
	newPassInput  textinput.Model
	focusIndex    int
	errMsg        string
	visible       bool
	width         int
	height        int
}

// NewModal creates a new modal.
func NewModal() Modal {
	ti := textinput.New()
	ti.CharLimit = 32
	ti.Width = 26

	cp := textinput.New()
	cp.Placeholder = "Current password"
	cp.EchoMode = textinput.EchoPassword
	cp.EchoCharacter = '•'
	cp.CharLimit = 64
	cp.Width = 26

	np := textinput.New()
	np.Placeholder = "New password (optional)"
	np.EchoMode = textinput.EchoPassword
	np.EchoCharacter = '•'
	np.CharLimit = 64
	np.Width = 26

	return Modal{
		input:         ti,
		currPassInput: cp,
		newPassInput:  np,
	}
}

// Show displays the modal.
func (m Modal) Show(t ModalType, title string) Modal {
	m.modalType = t
	m.title = title
	m.visible = true
	m.errMsg = ""
	m.input.SetValue("")

	switch t {
	case ModalCreateGuild:
		m.input.Placeholder = "Guild name"
		m.input.Focus()
	case ModalCreateChannel:
		m.input.Placeholder = "Channel name"
		m.input.Focus()
	case ModalHelp:
		m.input.Blur()
	}

	return m
}

// ShowSettings opens the settings modal for the user.
func (m Modal) ShowSettings(currentUsername string) Modal {
	m.modalType = ModalSettings
	m.title = "USER SETTINGS"
	m.visible = true
	m.errMsg = ""
	m.focusIndex = 0

	m.input.Placeholder = "New username"
	m.input.SetValue(currentUsername)
	m.input.Focus()

	m.currPassInput.SetValue("")
	m.currPassInput.Blur()

	m.newPassInput.SetValue("")
	m.newPassInput.Blur()

	return m
}

// SetError displays an error message inside the modal.
func (m Modal) SetError(err string) Modal {
	m.errMsg = err
	return m
}

func (m *Modal) updateSettingsFocus() {
	m.input.Blur()
	m.currPassInput.Blur()
	m.newPassInput.Blur()

	switch m.focusIndex {
	case 0:
		m.input.Focus()
	case 1:
		m.currPassInput.Focus()
	case 2:
		m.newPassInput.Focus()
	}
}

// Hide hides the modal.
func (m Modal) Hide() Modal {
	m.visible = false
	m.errMsg = ""
	m.input.Blur()
	m.currPassInput.Blur()
	m.newPassInput.Blur()
	return m
}

// IsVisible returns whether the modal is showing.
func (m Modal) IsVisible() bool {
	return m.visible
}

// Update handles modal input.
func (m Modal) Update(msg tea.Msg) (Modal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEsc || msg.String() == "esc" || (m.modalType == ModalHelp && (msg.String() == "q" || msg.String() == "?")) {
			return m.Hide(), nil
		}

		if m.modalType == ModalSettings {
			switch msg.String() {
			case "tab", "down":
				m.focusIndex = (m.focusIndex + 1) % 3
				m.updateSettingsFocus()
				return m, nil
			case "shift+tab", "up":
				m.focusIndex = (m.focusIndex - 1 + 3) % 3
				m.updateSettingsFocus()
				return m, nil
			case "enter", "\r", "\n", "ctrl+m":
				// If on first field and current password is empty, advance to current password field
				if m.focusIndex == 0 && strings.TrimSpace(m.currPassInput.Value()) == "" {
					m.focusIndex = 1
					m.updateSettingsFocus()
					return m, nil
				}
				newUname := strings.TrimSpace(m.input.Value())
				currPass := strings.TrimSpace(m.currPassInput.Value())
				newPass := strings.TrimSpace(m.newPassInput.Value())
				return m, func() tea.Msg {
					return SaveSettingsMsg{
						NewUsername:     newUname,
						CurrentPassword: currPass,
						NewPassword:     newPass,
					}
				}
			}

			var cmd tea.Cmd
			switch m.focusIndex {
			case 0:
				m.input, cmd = m.input.Update(msg)
			case 1:
				m.currPassInput, cmd = m.currPassInput.Update(msg)
			case 2:
				m.newPassInput, cmd = m.newPassInput.Update(msg)
			}
			return m, cmd
		}

		if msg.Type == tea.KeyEnter || msg.String() == "enter" || msg.String() == "\r" || msg.String() == "\n" || msg.String() == "ctrl+m" {
			if m.modalType == ModalHelp {
				return m.Hide(), nil
			}
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				return m, nil
			}
			m = m.Hide()
			switch m.modalType {
			case ModalCreateGuild:
				return m, func() tea.Msg { return CreateGuildMsg{Name: name} }
			case ModalCreateChannel:
				return m, func() tea.Msg { return CreateChannelMsg{Name: name} }
			}
			return m, nil
		}
	}

	if m.modalType != ModalHelp && m.modalType != ModalSettings {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View renders the modal.
func (m Modal) View() string {
	title := styles.TitleStyle.Render(m.title)

	if m.modalType == ModalSettings {
		var b strings.Builder

		header := lipgloss.NewStyle().
			Width(52).
			Align(lipgloss.Center).
			Render(title)
		b.WriteString(header + "\n\n")

		fieldLabel := func(label string, idx int) string {
			st := styles.MessageContent
			if m.focusIndex == idx {
				st = lipgloss.NewStyle().Bold(true).Foreground(styles.Primary)
			}
			return st.Render(fmt.Sprintf("  %-17s", label))
		}

		b.WriteString(fieldLabel("New Username:", 0) + m.input.View() + "\n\n")
		b.WriteString(fieldLabel("Current Password:", 1) + m.currPassInput.View() + "\n\n")
		b.WriteString(fieldLabel("New Password:", 2) + m.newPassInput.View() + "\n\n")

		if m.errMsg != "" {
			errText := lipgloss.NewStyle().
				Foreground(styles.Error).
				Bold(true).
				Render("  " + m.errMsg)
			b.WriteString(errText + "\n\n")
		}

		footer := lipgloss.NewStyle().
			Width(52).
			Align(lipgloss.Center).
			Render(styles.HelpStyle.Render("[Tab] Next Field  │  [Enter] Save  │  [Esc] Cancel"))
		b.WriteString(footer)

		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			Padding(1, 2).
			Width(58).
			Render(b.String())
	}

	if m.modalType == ModalHelp {
		var b strings.Builder

		// Centered Title
		title := lipgloss.NewStyle().
			Bold(true).
			Foreground(styles.Cyan).
			Render("SHELL CHAT — QUICK GUIDE")

		header := lipgloss.NewStyle().
			Width(56).
			Align(lipgloss.Center).
			Render(title)
		b.WriteString(header + "\n\n")

		section := func(name string) string {
			return lipgloss.NewStyle().Bold(true).Foreground(styles.Accent).Render(name) + "\n"
		}
		row := func(key, desc string) string {
			k := lipgloss.NewStyle().Bold(true).Foreground(styles.Primary).Render(fmt.Sprintf("%-15s", key))
			d := styles.MessageContent.Render(desc)
			return fmt.Sprintf("  %s %s\n", k, d)
		}

		b.WriteString(section("NAVIGATION"))
		b.WriteString(row("Tab", "Cycle focus (Input / Channels / Online)"))
		b.WriteString(row("Up / Down", "Select channel or online member"))
		b.WriteString(row("Enter", "Send message / Open channel or DM"))
		b.WriteString(row("Esc", "Focus chat input / Return to bottom"))
		b.WriteString(row("PgUp / PgDn", "Scroll message history up / down\n"))

		b.WriteString(section("COMMANDS"))
		b.WriteString(row("/ask <query>", "Ask Spark AI (with rolling memory)"))
		b.WriteString(row("/calc <expr>", "Fast in-terminal math calculator"))
		b.WriteString(row("/search <kw>", "Search past message history"))
		b.WriteString(row("/tz <offset>", "Set timezone (e.g. /tz IST, /tz +5:30)"))
		b.WriteString(row("/settings", "Change username & password"))
		b.WriteString(row("Ctrl+C", "Quit application\n"))

		footer := lipgloss.NewStyle().
			Width(56).
			Align(lipgloss.Center).
			Render(styles.HelpStyle.Render("[Press Esc or Enter to close]"))
		b.WriteString(footer)

		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(styles.Primary).
			Padding(1, 2).
			Width(62).
			Render(b.String())
	}

	content := title + "\n\n" +
		styles.MessageContent.Render("  Name: ") + m.input.View() + "\n\n" +
		styles.HelpStyle.Render("  [Enter] Submit  [Esc] Cancel")

	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 3).
		Width(48).
		Render(content)
}
