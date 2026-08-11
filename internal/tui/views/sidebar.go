package views

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// ChannelSelectedMsg is emitted when a channel is selected.
type ChannelSelectedMsg struct {
	Guild   *models.Guild
	Channel *models.Channel
}

// GuildSelectedMsg is emitted when a guild is selected.
type GuildSelectedMsg struct {
	Guild *models.Guild
}

// Sidebar displays guilds and channels.
type Sidebar struct {
	guilds          []*models.Guild
	channels        []*models.Channel
	unreadChannels  map[int64]bool
	activeChannelID int64
	selectedGuild   int
	selectedChannel int
	focused         bool
	width           int
	height          int
}

// NewSidebar creates a new sidebar.
func NewSidebar() Sidebar {
	return Sidebar{
		unreadChannels: make(map[int64]bool),
	}
}

// SetActiveChannel sets which channel is currently open and active.
func (s Sidebar) SetActiveChannel(id int64) Sidebar {
	s.activeChannelID = id
	for i, ch := range s.channels {
		if ch.ID == id {
			s.selectedChannel = i
			break
		}
	}
	return s
}

// MarkUnread marks a channel as having unread messages.
func (s Sidebar) MarkUnread(channelID int64) Sidebar {
	if s.unreadChannels == nil {
		s.unreadChannels = make(map[int64]bool)
	}
	s.unreadChannels[channelID] = true
	return s
}

// ClearUnread clears the unread badge for a channel.
func (s Sidebar) ClearUnread(channelID int64) Sidebar {
	if s.unreadChannels != nil {
		delete(s.unreadChannels, channelID)
	}
	return s
}

// FocusInputMsg is emitted when sidebar returns focus to input.
type FocusInputMsg struct{}

// Update handles input for the sidebar.
func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if !s.focused {
			return s, nil
		}
		switch msg.String() {
		case "up", "k":
			if s.selectedChannel > 0 {
				s.selectedChannel--
			}
		case "down", "j":
			if s.selectedChannel < len(s.channels)-1 {
				s.selectedChannel++
			}
		case "enter":
			if s.selectedChannel < len(s.channels) {
				return s, func() tea.Msg {
					var guild *models.Guild
					if s.selectedGuild < len(s.guilds) {
						guild = s.guilds[s.selectedGuild]
					}
					return ChannelSelectedMsg{
						Guild:   guild,
						Channel: s.channels[s.selectedChannel],
					}
				}
			}
		case "esc", "tab", "right", "l":
			return s, func() tea.Msg {
				return FocusInputMsg{}
			}
		}
	}
	return s, nil
}

// View renders the sidebar.
func (s Sidebar) View() string {
	var b strings.Builder

	// Header
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.Accent).
		Render("  ⚡ SHELL CHAT")
	b.WriteString(header + "\n")

	// Separator
	sep := styles.HelpStyle.Render("  " + strings.Repeat("─", max(0, s.width-4)))
	b.WriteString(sep + "\n\n")

	// Channels header
	chHeading := lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.TextDim).
		Render("  CHANNELS")
	b.WriteString(chHeading + "\n")

	// Channels list
	for j, ch := range s.channels {
		hasUnread := s.unreadChannels != nil && s.unreadChannels[ch.ID]

		var isHighlighted bool
		if s.focused {
			isHighlighted = (j == s.selectedChannel)
		} else {
			isHighlighted = (s.activeChannelID != 0 && ch.ID == s.activeChannelID)
		}

		var unreadDot string
		if hasUnread {
			// Blue dot on the RIGHT side of the channel name!
			unreadDot = " " + lipgloss.NewStyle().Foreground(styles.Primary).Bold(true).Render("●")
		}

		icon := lipgloss.NewStyle().Foreground(styles.Cyan).Bold(true).Render("#")
		if ch.Type == models.ChannelTypeAnnouncement {
			icon = lipgloss.NewStyle().Foreground(styles.Pink).Bold(true).Render("📢")
		}

		if isHighlighted {
			prefix := "#"
			if ch.Type == models.ChannelTypeAnnouncement {
				prefix = "📢"
			}
			badge := lipgloss.NewStyle().
				Foreground(styles.TextBright).
				Background(styles.PrimaryDark).
				Bold(true).
				Render(fmt.Sprintf(" ▶ %s %s%s", prefix, ch.Name, unreadDot))
			b.WriteString(" " + badge + "\n")
		} else {
			name := lipgloss.NewStyle().Foreground(styles.Text).Render(ch.Name)
			b.WriteString(fmt.Sprintf("   %s %s%s\n", icon, name, unreadDot))
		}
	}

	if len(s.channels) == 0 {
		b.WriteString(styles.HelpStyle.Render("  No channels\n"))
	}

	return b.String()
}

func (s Sidebar) SetSize(w, h int) Sidebar            { s.width = w; s.height = h; return s }
func (s Sidebar) SetGuilds(g []*models.Guild) Sidebar { s.guilds = g; return s }
func (s Sidebar) SetChannels(c []*models.Channel) Sidebar {
	s.channels = c
	s.selectedChannel = 0
	return s
}
func (s Sidebar) SetFocused(f bool) Sidebar {
	s.focused = f
	if f && s.activeChannelID != 0 {
		for i, ch := range s.channels {
			if ch.ID == s.activeChannelID {
				s.selectedChannel = i
				break
			}
		}
	}
	return s
}

func (s Sidebar) SelectedGuildID() int64 {
	if s.selectedGuild < len(s.guilds) {
		return s.guilds[s.selectedGuild].ID
	}
	return 0
}

func (s Sidebar) SelectedChannelID() int64 {
	if s.selectedChannel < len(s.channels) {
		return s.channels[s.selectedChannel].ID
	}
	return 0
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
