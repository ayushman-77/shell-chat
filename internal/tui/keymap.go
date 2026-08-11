package tui

import "github.com/charmbracelet/bubbles/key"

// KeyMap defines all keyboard shortcuts for the application.
type KeyMap struct {
	Quit          key.Binding
	Send          key.Binding
	ToggleFocus   key.Binding
	FocusSidebar  key.Binding
	FocusChat     key.Binding
	FocusInput    key.Binding
	ScrollUp      key.Binding
	ScrollDown    key.Binding
	Help          key.Binding
	CreateGuild   key.Binding
	CreateChannel key.Binding
	Escape        key.Binding
}

// Keys is the default key binding configuration.
var Keys = KeyMap{
	Quit:          key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit")),
	Send:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
	ToggleFocus:   key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch focus")),
	FocusSidebar:  key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "channels")),
	FocusChat:     key.NewBinding(key.WithKeys("ctrl+l"), key.WithHelp("ctrl+l", "chat")),
	FocusInput:    key.NewBinding(key.WithKeys("ctrl+j"), key.WithHelp("ctrl+j", "input")),
	ScrollUp:      key.NewBinding(key.WithKeys("pgup", "ctrl+u", "shift+up"), key.WithHelp("pgup/ctrl+u", "scroll up")),
	ScrollDown:    key.NewBinding(key.WithKeys("pgdown", "ctrl+d", "shift+down"), key.WithHelp("pgdn/ctrl+d", "scroll down")),
	Help:          key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
	CreateGuild:   key.NewBinding(key.WithKeys("ctrl+g"), key.WithHelp("ctrl+g", "create server")),
	CreateChannel: key.NewBinding(key.WithKeys("ctrl+n"), key.WithHelp("ctrl+n", "new channel")),
	Escape:        key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "focus input")),
}

