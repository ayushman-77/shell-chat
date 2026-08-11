package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// SendMessageMsg is emitted when the user presses Enter to send a message.
type SendMessageMsg struct {
	Content string
}

// Input handles message composition.
type Input struct {
	textInput   textinput.Model
	focused     bool
	disabled    bool
	width       int
	placeholder string
}

// NewInput creates a new input component.
func NewInput() Input {
	ti := textinput.New()
	ti.Prompt = "💬 "
	ti.Placeholder = "Type a message and press Enter to send..."
	ti.CharLimit = 2000
	ti.Width = 60

	return Input{
		textInput:   ti,
		placeholder: "Type a message and press Enter to send...",
	}
}

// SetDisabled sets whether the input is disabled (read-only).
func (i Input) SetDisabled(d bool) Input {
	i.disabled = d
	if d {
		i.focused = false
		i.textInput.Blur()
		i.textInput.SetValue("")
	}
	return i
}

// IsDisabled returns whether the input is disabled.
func (i Input) IsDisabled() bool {
	return i.disabled
}

// Update handles input events.
func (i Input) Update(msg tea.Msg) (Input, tea.Cmd) {
	if i.disabled {
		return i, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyEnter || msg.String() == "enter" || msg.String() == "\r" || msg.String() == "\n" || msg.String() == "ctrl+m" {
			content := strings.TrimSpace(i.textInput.Value())
			if content != "" {
				i.textInput.SetValue("")
				return i, func() tea.Msg {
					return SendMessageMsg{Content: content}
				}
			}
			return i, nil
		}
	}

	var cmd tea.Cmd
	i.textInput, cmd = i.textInput.Update(msg)
	return i, cmd
}

// View renders the input component.
func (i Input) View() string {
	w := i.width - 4
	if w < 10 {
		w = 10
	}

	if i.disabled {
		text := styles.HelpStyle.Render("💬 " + i.placeholder)
		return styles.InputStyle.Width(w).Render(text)
	}

	var style = styles.InputStyle
	if i.focused {
		style = styles.InputFocused
	}
	return style.Width(w).Render(i.textInput.View())
}

// SetSize updates the input width.
func (i Input) SetSize(w int) Input {
	i.width = w
	tiWidth := w - 8
	if tiWidth < 10 {
		tiWidth = 10
	}
	i.textInput.Width = tiWidth
	return i
}

// Focus focuses the input.
func (i Input) Focus() Input {
	if i.disabled {
		i.focused = false
		i.textInput.Blur()
		return i
	}
	i.focused = true
	i.textInput.Focus()
	return i
}

// Blur removes focus from the input.
func (i Input) Blur() Input {
	i.focused = false
	i.textInput.Blur()
	return i
}

// Value returns the current input value.
func (i Input) Value() string {
	return i.textInput.Value()
}

// SetPlaceholder updates the input placeholder text.
func (i Input) SetPlaceholder(p string) Input {
	i.placeholder = p
	i.textInput.Placeholder = p
	return i
}
