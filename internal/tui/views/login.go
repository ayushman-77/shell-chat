package views

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/crypto/bcrypt"

	"github.com/ayushman-77/shell-chat/internal/models"
	sfgen "github.com/ayushman-77/shell-chat/internal/snowflake"
	"github.com/ayushman-77/shell-chat/internal/storage"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

type loginState int

const (
	stateUsername loginState = iota
	statePassword
	stateRegisterPassword
	stateRegisterConfirm
	stateRegisterDisplayName
)

// LoginSuccessMsg is emitted when authentication succeeds.
type LoginSuccessMsg struct{ User *models.User }

// LoginErrorMsg is emitted when authentication fails.
type LoginErrorMsg struct{ Err error }

// LoginView handles user login and registration.
type LoginView struct {
	state       loginState
	username    textinput.Model
	password    textinput.Model
	confirm     textinput.Model
	displayName textinput.Model

	userStore *storage.UserStore
	errMsg    string
	width     int
	height    int
	isNewUser bool
}

// NewLoginView creates a new login view.
func NewLoginView(username string, userStore *storage.UserStore) LoginView {
	ui := textinput.New()
	ui.Placeholder = "Enter username"
	ui.CharLimit = 32
	ui.Width = 30

	pw := textinput.New()
	pw.Placeholder = "Enter password"
	pw.EchoMode = textinput.EchoPassword
	pw.EchoCharacter = '•'
	pw.CharLimit = 128
	pw.Width = 30

	conf := textinput.New()
	conf.Placeholder = "Confirm password"
	conf.EchoMode = textinput.EchoPassword
	conf.EchoCharacter = '•'
	conf.CharLimit = 128
	conf.Width = 30

	dn := textinput.New()
	dn.Placeholder = "Choose display name"
	dn.CharLimit = 32
	dn.Width = 30

	l := LoginView{
		state:       stateUsername,
		username:    ui,
		password:    pw,
		confirm:     conf,
		displayName: dn,
		userStore:   userStore,
	}

	// Suggest default username if present, but keep field focused and editable
	if username != "" && !strings.HasPrefix(username, "u0_") {
		l.username.SetValue(username)
		l.displayName.SetValue(username)
	}

	l.username.Focus()
	return l
}

// Init returns the initial command for the login view.
func (l LoginView) Init() tea.Cmd {
	return textinput.Blink
}

// isEnterKey checks if a KeyMsg represents an Enter keypress across all terminal emulators.
func isEnterKey(msg tea.KeyMsg) bool {
	return msg.Type == tea.KeyEnter ||
		msg.String() == "enter" ||
		msg.String() == "\r" ||
		msg.String() == "\n" ||
		msg.String() == "ctrl+m"
}

// Update handles input for the login view.
func (l LoginView) Update(msg tea.Msg) (LoginView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if isEnterKey(msg) {
			return l.handleEnter()
		}
		// Esc returns to Username field so user can switch account
		if msg.Type == tea.KeyEsc || msg.String() == "esc" {
			if l.state != stateUsername {
				l.state = stateUsername
				l.password.Blur()
				l.confirm.Blur()
				l.displayName.Blur()
				l.password.SetValue("")
				l.confirm.SetValue("")
				l.username.Focus()
				l.errMsg = ""
				return l, nil
			}
		}
		if msg.Type == tea.KeyTab || msg.String() == "tab" {
			return l, nil
		}
	}

	// Forward to active input
	var cmd tea.Cmd
	switch l.state {
	case stateUsername:
		l.username, cmd = l.username.Update(msg)
	case statePassword:
		l.password, cmd = l.password.Update(msg)
	case stateRegisterPassword:
		l.password, cmd = l.password.Update(msg)
	case stateRegisterConfirm:
		l.confirm, cmd = l.confirm.Update(msg)
	case stateRegisterDisplayName:
		l.displayName, cmd = l.displayName.Update(msg)
	}

	return l, cmd
}

func (l LoginView) handleEnter() (LoginView, tea.Cmd) {
	switch l.state {
	case stateUsername:
		username := strings.TrimSpace(l.username.Value())
		if username == "" {
			l.errMsg = "Username cannot be empty"
			return l, nil
		}
		l.displayName.SetValue(username)

		// Check if user exists in database
		exists, _ := l.userStore.UsernameExists(context.Background(), username)
		if exists {
			l.isNewUser = false
			l.state = statePassword
			l.password.Placeholder = "Enter your password"
			l.password.SetValue("")
			l.username.Blur()
			l.password.Focus()
			l.errMsg = "Welcome back! Enter your password."
			return l, nil
		}

		l.isNewUser = true
		l.state = stateRegisterPassword
		l.password.Placeholder = "Choose a password"
		l.password.SetValue("")
		l.username.Blur()
		l.password.Focus()
		l.errMsg = "New account! Choose a password to register."
		return l, nil

	case statePassword:
		password := l.password.Value()
		if password == "" {
			l.errMsg = "Password cannot be empty"
			return l, nil
		}

		// Verify password
		return l, l.attemptLogin()

	case stateRegisterPassword:
		if l.password.Value() == "" {
			l.errMsg = "Password cannot be empty"
			return l, nil
		}
		l.state = stateRegisterConfirm
		l.password.Blur()
		l.confirm.SetValue("")
		l.confirm.Focus()
		l.errMsg = ""
		return l, nil

	case stateRegisterConfirm:
		if l.confirm.Value() != l.password.Value() {
			l.errMsg = "Passwords do not match"
			l.confirm.SetValue("")
			return l, nil
		}
		l.state = stateRegisterDisplayName
		l.confirm.Blur()
		l.displayName.Focus()
		if l.displayName.Value() == "" {
			l.displayName.SetValue(l.username.Value())
		}
		l.errMsg = ""
		return l, nil

	case stateRegisterDisplayName:
		dn := strings.TrimSpace(l.displayName.Value())
		if dn == "" {
			dn = l.username.Value()
		}
		return l, l.registerUser(dn)
	}

	return l, nil
}

func (l LoginView) attemptLogin() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		user, err := l.userStore.GetUserByUsername(ctx, l.username.Value())
		if err != nil {
			return LoginErrorMsg{Err: fmt.Errorf("user not found")}
		}

		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(l.password.Value())); err != nil {
			return LoginErrorMsg{Err: fmt.Errorf("invalid password (press Esc to change username)")}
		}

		return LoginSuccessMsg{User: user}
	}
}

func (l LoginView) registerUser(displayName string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		user := &models.User{
			ID:           sfgen.Generate(),
			Username:     l.username.Value(),
			DisplayName:  displayName,
			PasswordHash: l.password.Value(), // CreateUser will hash this
			Status:       models.StatusOnline,
		}

		if err := l.userStore.CreateUser(ctx, user); err != nil {
			return LoginErrorMsg{Err: fmt.Errorf("registration failed: %w", err)}
		}

		return LoginSuccessMsg{User: user}
	}
}

// View renders the login view.
func (l LoginView) View() string {
	var b strings.Builder

	// Centered Logo & Subtitle
	logo := styles.TitleStyle.Render("⚡ SHELL CHAT")
	subtitle := styles.HelpStyle.Render("Terminal-native chat over SSH")
	headerBlock := lipgloss.NewStyle().
		Width(48).
		Align(lipgloss.Center).
		Render(logo + "\n" + subtitle)

	b.WriteString(headerBlock)
	b.WriteString("\n\n")

	// Input fields based on state
	b.WriteString(styles.MessageContent.Render("  Username: "))
	b.WriteString(l.username.View())
	b.WriteString("\n\n")

	if l.state >= statePassword {
		b.WriteString(styles.MessageContent.Render("  Password: "))
		b.WriteString(l.password.View())
		b.WriteString("\n\n")
	}

	if l.state >= stateRegisterConfirm {
		b.WriteString(styles.MessageContent.Render("  Confirm:  "))
		b.WriteString(l.confirm.View())
		b.WriteString("\n\n")
	}

	if l.state >= stateRegisterDisplayName {
		b.WriteString(styles.MessageContent.Render("  Display:  "))
		b.WriteString(l.displayName.View())
		b.WriteString("\n\n")
	}

	// Error / Status Message
	if l.errMsg != "" {
		b.WriteString("  ")
		if strings.HasPrefix(l.errMsg, "New") || strings.HasPrefix(l.errMsg, "Welcome") {
			b.WriteString(styles.HelpStyle.Render(l.errMsg))
		} else {
			b.WriteString(styles.ErrorStyle.Render(l.errMsg))
		}
		b.WriteString("\n\n")
	}

	// Centered Help Footer
	var helpText string
	if l.state == stateUsername {
		helpText = "Press [Enter] to continue"
	} else {
		helpText = "[Enter]: continue  │  [Esc]: change username"
	}
	footer := lipgloss.NewStyle().
		Width(48).
		Align(lipgloss.Center).
		Render(styles.HelpStyle.Render(helpText))
	b.WriteString(footer)

	// Wrap in a box
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		Padding(1, 3).
		Width(56).
		Render(b.String())

	return box
}

// SetSize updates the view dimensions.
func (l LoginView) SetSize(w, h int) LoginView {
	l.width = w
	l.height = h
	return l
}

// SetError sets the error message.
func (l LoginView) SetError(msg string) LoginView {
	l.errMsg = msg
	return l
}
