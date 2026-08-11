package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/actor"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// DMSelectedMsg is emitted when a user selects a DM or online member to chat with.
type DMSelectedMsg struct {
	UserID   int64
	Username string
}

// DMConversation represents a 1-on-1 direct message conversation.
type DMConversation struct {
	UserID   int64
	Username string
	Unread   bool
}

// MembersView displays online users and active DM conversations in the right sidebar.
type MembersView struct {
	onlineUsers    []actor.OnlineUser
	dms            []DMConversation
	activeDMUserID int64
	focused        bool
	cursor         int
	width          int
	height         int
}

// NewMembersView creates a new members view.
func NewMembersView(w, h int) MembersView {
	return MembersView{
		width:  w,
		height: h,
	}
}

// SetActiveDM sets which user's DM is currently open.
func (m MembersView) SetActiveDM(userID int64) MembersView {
	m.activeDMUserID = userID
	return m
}

// SetUsers updates the online users list.
func (m MembersView) SetUsers(users []actor.OnlineUser) MembersView {
	m.onlineUsers = users
	return m
}

// SetFocused sets whether the right sidebar has keyboard focus.
func (m MembersView) SetFocused(f bool) MembersView {
	m.focused = f
	if f && m.activeDMUserID != 0 {
		for j, dm := range m.dms {
			if dm.UserID == m.activeDMUserID {
				m.cursor = len(m.onlineUsers) + j
				break
			}
		}
	}
	return m
}

// SetSize updates the view dimensions.
func (m MembersView) SetSize(w, h int) MembersView {
	m.width = w
	m.height = h
	return m
}

// AddOrUpdateDM adds a DM conversation or marks it unread.
func (m MembersView) AddOrUpdateDM(userID int64, username string, unread bool) MembersView {
	for i, dm := range m.dms {
		if dm.UserID == userID {
			m.dms[i].Username = username
			if unread {
				m.dms[i].Unread = true
			}
			return m
		}
	}
	// Add new DM conversation
	m.dms = append(m.dms, DMConversation{
		UserID:   userID,
		Username: username,
		Unread:   unread,
	})
	return m
}

// UpdateUsername updates the username in DM list.
func (m MembersView) UpdateUsername(userID int64, newUsername string) MembersView {
	for i, dm := range m.dms {
		if dm.UserID == userID {
			m.dms[i].Username = newUsername
		}
	}
	return m
}

// ClearUnread clears the unread notification badge for a specific user.
func (m MembersView) ClearUnread(userID int64) MembersView {
	for i, dm := range m.dms {
		if dm.UserID == userID {
			m.dms[i].Unread = false
			return m
		}
	}
	return m
}

// totalItems returns the total number of selectable items (online users + DMs).
func (m MembersView) totalItems() int {
	return len(m.onlineUsers) + len(m.dms)
}

// Update handles keyboard navigation in the right sidebar.
func (m MembersView) Update(msg tea.Msg) (MembersView, tea.Cmd) {
	total := m.totalItems()
	if total == 0 {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < total-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor < len(m.onlineUsers) {
				u := m.onlineUsers[m.cursor]
				return m, func() tea.Msg {
					return DMSelectedMsg{
						UserID:   u.UserID,
						Username: u.Username,
					}
				}
			}
			dmIdx := m.cursor - len(m.onlineUsers)
			if dmIdx >= 0 && dmIdx < len(m.dms) {
				dm := m.dms[dmIdx]
				return m, func() tea.Msg {
					return DMSelectedMsg{
						UserID:   dm.UserID,
						Username: dm.Username,
					}
				}
			}
		}
	}

	return m, nil
}

// View renders the right sidebar.
func (m MembersView) View() string {
	var b strings.Builder

	// 1. ONLINE SECTION
	onlineHeading := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextDim).
		Render("  ONLINE")
	b.WriteString(onlineHeading + "\n")

	sep := styles.HelpStyle.Render("  " + strings.Repeat("─", max(0, m.width-4)))
	b.WriteString(sep + "\n")

	for i, u := range m.onlineUsers {
		dot := lipgloss.NewStyle().Foreground(styles.Success).Render("●")
		var line string
		if m.focused && m.cursor == i {
			line = lipgloss.NewStyle().
				Foreground(styles.TextBright).
				Background(styles.PrimaryDark).
				Bold(true).
				Render(fmt.Sprintf(" ▶ ● %s", u.Username))
		} else {
			line = fmt.Sprintf("   %s %s", dot, lipgloss.NewStyle().Foreground(styles.TextBright).Render(u.Username))
		}
		b.WriteString(line + "\n")
	}

	if len(m.onlineUsers) == 0 {
		b.WriteString(styles.HelpStyle.Render("  None online\n"))
	}

	// 2. DIRECT MESSAGES SECTION
	b.WriteString("\n")
	dmHeading := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextDim).
		Render("  DIRECT MESSAGES")
	b.WriteString(dmHeading + "\n")
	b.WriteString(sep + "\n")

	for j, dm := range m.dms {
		globalIdx := len(m.onlineUsers) + j
		var icon string = lipgloss.NewStyle().Foreground(styles.TextDim).Render("@")

		var isHighlighted bool
		if m.focused {
			isHighlighted = (m.cursor == globalIdx)
		} else {
			isHighlighted = (m.activeDMUserID != 0 && dm.UserID == m.activeDMUserID)
		}

		var unreadDot string
		if dm.Unread {
			// Blue dot on the RIGHT side of the username!
			unreadDot = " " + lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("●")
		}

		var line string
		if isHighlighted {
			line = lipgloss.NewStyle().
				Foreground(styles.TextBright).
				Background(styles.PrimaryDark).
				Bold(true).
				Render(fmt.Sprintf(" ▶ %s %s%s", icon, dm.Username, unreadDot))
		} else {
			line = fmt.Sprintf("   %s %s%s", icon, lipgloss.NewStyle().Foreground(styles.TextBright).Render(dm.Username), unreadDot)
		}
		b.WriteString(line + "\n")
	}

	if len(m.dms) == 0 {
		b.WriteString(styles.HelpStyle.Render("  No direct messages\n"))
	}

	return b.String()
}
