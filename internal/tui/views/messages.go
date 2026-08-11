package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/tui/styles"
)

// MessageView displays chat messages with scrolling.
type MessageView struct {
	viewport     viewport.Model
	messages     []*models.Message
	usernames    map[int64]string
	channelName  string
	channelTopic string
	typing       []string
	width        int
	height       int
	atBottom     bool
	focused      bool
	location     *time.Location
}

// NewMessageView creates a new message view.
func NewMessageView(w, h int) MessageView {
	vp := viewport.New(w, h)
	vp.SetContent("Welcome to Shell Chat!\n\nSelect a channel to start chatting.")

	return MessageView{
		viewport:  vp,
		usernames: make(map[int64]string),
		width:     w,
		height:    h,
		atBottom:  true,
		focused:   false,
		location:  time.Local,
	}
}

// SetFocused sets whether the message view has active keyboard scroll focus.
func (m MessageView) SetFocused(f bool) MessageView {
	m.focused = f
	return m
}

// LineUp scrolls up by n lines.
func (m MessageView) LineUp(n int) MessageView {
	m.viewport.LineUp(n)
	m.atBottom = m.viewport.AtBottom()
	return m
}

// LineDown scrolls down by n lines.
func (m MessageView) LineDown(n int) MessageView {
	m.viewport.LineDown(n)
	m.atBottom = m.viewport.AtBottom()
	return m
}

// PageUp scrolls up by one page.
func (m MessageView) PageUp() MessageView {
	m.viewport.ViewUp()
	m.atBottom = m.viewport.AtBottom()
	return m
}

// PageDown scrolls down by one page.
func (m MessageView) PageDown() MessageView {
	m.viewport.ViewDown()
	m.atBottom = m.viewport.AtBottom()
	return m
}

// GotoTop scrolls to the top of message history.
func (m MessageView) GotoTop() MessageView {
	m.viewport.GotoTop()
	m.atBottom = m.viewport.AtBottom()
	return m
}

// GotoBottom scrolls to the latest messages at the bottom.
func (m MessageView) GotoBottom() MessageView {
	m.viewport.GotoBottom()
	m.atBottom = true
	return m
}

// SetLocation updates the display timezone.
func (m MessageView) SetLocation(loc *time.Location) MessageView {
	m.location = loc
	m.viewport.SetContent(m.renderMessages())
	return m
}

// Update handles viewport scrolling and messages.
func (m MessageView) Update(msg tea.Msg) (MessageView, tea.Cmd) {
	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	m.atBottom = m.viewport.AtBottom()
	return m, cmd
}

// View renders the message view.
func (m MessageView) View() string {
	// Channel header
	header := ""
	if m.channelName != "" {
		topic := "Global community channel"
		if m.channelTopic != "" {
			topic = m.channelTopic
		}

		chBadge := styles.TitleStyle.Render(fmt.Sprintf(" # %s ", m.channelName))
		topicBadge := styles.HelpStyle.Render(fmt.Sprintf(" │  💬 %s", topic))
		liveBadge := styles.StatusBarOnline.Render("  ● LIVE")

		header = fmt.Sprintf("  %s%s%s\n  %s\n",
			chBadge,
			topicBadge,
			liveBadge,
			styles.HelpStyle.Render(strings.Repeat("─", max(10, m.width-4))),
		)
	}

	return header + m.viewport.View()
}

// SetSize updates the viewport dimensions.
func (m MessageView) SetSize(w, h int) MessageView {
	m.width = w
	m.height = h
	headerHeight := 3
	if m.channelName == "" {
		headerHeight = 0
	}
	m.viewport.Width = w
	m.viewport.Height = max(1, h-headerHeight)
	m.viewport.SetContent(m.renderMessages())
	return m
}

// SetMessages replaces all messages and re-renders.
func (m MessageView) SetMessages(msgs []*models.Message) MessageView {
	// Storage returns DESC (newest first). Reverse into ASC (oldest first) with deduplication.
	seen := make(map[int64]bool)
	var unique []*models.Message
	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]
		if msg.ID != 0 && seen[msg.ID] {
			continue
		}
		if msg.ID != 0 {
			seen[msg.ID] = true
		}
		unique = append(unique, msg)
	}
	m.messages = unique
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m
}

// UpdateUsername updates the author name for a given user ID and re-renders all messages.
func (m MessageView) UpdateUsername(userID int64, newUsername string) MessageView {
	if m.usernames == nil {
		m.usernames = make(map[int64]string)
	}
	m.usernames[userID] = newUsername
	for _, msg := range m.messages {
		if msg.AuthorID == userID {
			msg.AuthorName = newUsername
		}
	}
	m.viewport.SetContent(m.renderMessages())
	if m.atBottom {
		m.viewport.GotoBottom()
	}
	return m
}

// AddMessage appends a single message and re-renders in chronological order.
func (m MessageView) AddMessage(msg *models.Message) MessageView {
	// Deduplicate if already present by ID or exact content within 2 seconds
	for _, existing := range m.messages {
		if existing.ID == msg.ID {
			return m
		}
		if existing.AuthorID == msg.AuthorID && existing.Content == msg.Content {
			diff := existing.CreatedAt.Sub(msg.CreatedAt)
			if diff < 0 {
				diff = -diff
			}
			if diff < 2*time.Second {
				return m
			}
		}
	}

	// Insert in chronological ASC order by Snowflake ID
	inserted := false
	for i, existing := range m.messages {
		if msg.ID != 0 && existing.ID != 0 && msg.ID < existing.ID {
			m.messages = append(m.messages[:i], append([]*models.Message{msg}, m.messages[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		m.messages = append(m.messages, msg)
	}

	m.typing = nil // clear typing when message arrives
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m
}

// AddSystemMessage appends a system notification.
func (m MessageView) AddSystemMessage(content string) MessageView {
	sysMsg := &models.Message{
		Content:   content,
		CreatedAt: time.Now(),
	}
	m.messages = append(m.messages, sysMsg)
	m.viewport.SetContent(m.renderMessages())
	m.viewport.GotoBottom()
	return m
}

// AddTyping adds a username to the typing indicator.
func (m MessageView) AddTyping(username string) MessageView {
	// Check if already typing
	for _, u := range m.typing {
		if u == username {
			return m
		}
	}
	m.typing = append(m.typing, username)
	m.viewport.SetContent(m.renderMessages())
	return m
}

// SetChannel updates the channel header.
func (m MessageView) SetChannel(name, topic string) MessageView {
	m.channelName = name
	m.channelTopic = topic
	m.messages = nil
	m.typing = nil
	m.viewport.SetContent(m.renderMessages())
	return m
}

// SetUsername caches a username for display.
func (m MessageView) SetUsername(userID int64, name string) MessageView {
	m.usernames[userID] = name
	return m
}

func (m MessageView) renderMessages() string {
	var b strings.Builder
	var lastAuthor int64
	usableWidth := max(20, m.viewport.Width-4)

	// If empty channel (0 messages), display initial welcome box
	if len(m.messages) == 0 {
		if m.channelName == "announcements" {
			annHeader := lipgloss.NewStyle().
				Width(usableWidth).
				Align(lipgloss.Center).
				Render(
					styles.TitleStyle.Render("📢 Welcome to #announcements") + "\n" +
						styles.HelpStyle.Render("Official server updates, member joins, and announcements."),
				)
			return "\n" + annHeader + "\n\n" + styles.HelpStyle.Render(strings.Repeat("─", usableWidth)) + "\n\n"
		} else if m.channelName != "" {
			boxWidth := min(56, max(20, m.width-8))
			title := "👋 Welcome to #" + m.channelName + "!"
			desc := "This is the start of the chat room."
			if strings.HasPrefix(m.channelName, "@") {
				title = "👋 " + m.channelName
				desc = "This is the start of your direct message history with " + m.channelName + "."
			}
			welcomeBox := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(styles.Accent).
				Padding(1, 2).
				Width(boxWidth).
				Render(fmt.Sprintf(
					"%s\n%s\n\n%s",
					styles.TitleStyle.Render(title),
					styles.MessageContent.Render(desc),
					styles.HelpStyle.Render("Type below and press Enter to chat with everyone!"),
				))
			return "\n  " + strings.ReplaceAll(welcomeBox, "\n", "\n  ") + "\n\n"
		}
		return ""
	}

	// 1. Channel Welcome Banner (Always at the top for #announcements ONLY)
	if m.channelName == "announcements" {
		annHeader := lipgloss.NewStyle().
			Width(usableWidth).
			Align(lipgloss.Center).
			Render(
				styles.TitleStyle.Render("📢 Welcome to #announcements") + "\n" +
					styles.HelpStyle.Render("Official server updates, member joins, and announcements."),
			)
		b.WriteString("\n" + annHeader + "\n\n" + styles.HelpStyle.Render(strings.Repeat("─", usableWidth)) + "\n\n")
	}

	// 2. Messages stored in chronological ASC order (oldest first)
	for _, msg := range m.messages {
		timestamp := m.formatTimestamp(msg.CreatedAt)

		// Announcements Channel / System Announcements — Centered, fairly spaced, scrollable
		if m.channelName == "announcements" || (msg.AuthorID == models.SparkBotID && strings.HasPrefix(msg.AuthorName, "📢")) {
			annText := msg.Content
			annText = strings.ReplaceAll(annText, "**", "")
			annText = strings.TrimPrefix(annText, "📢 ")

			timeStr := styles.MessageTime.Render(fmt.Sprintf("[%s]", timestamp))
			card := lipgloss.NewStyle().
				Width(usableWidth).
				Align(lipgloss.Center).
				Render(
					lipgloss.NewStyle().Bold(true).Foreground(styles.Cyan).Render("📢 "+annText) + "  " + timeStr,
				)
			b.WriteString(card + "\n\n")
			lastAuthor = 0
			continue
		}

		// System message with word wrapping and generous spacing
		if msg.AuthorID == 0 {
			b.WriteString("\n")
			sysWidth := max(10, m.viewport.Width-6)
			wrappedSys := lipgloss.NewStyle().
				Width(sysWidth).
				Foreground(styles.TextDim).
				Render(msg.Content)

			lines := strings.Split(wrappedSys, "\n")
			for i, line := range lines {
				if i == 0 {
					b.WriteString(fmt.Sprintf("  ⚡ %s\n", line))
				} else {
					b.WriteString(fmt.Sprintf("     %s\n", line))
				}
			}
			b.WriteString("\n")
			lastAuthor = 0
			continue
		}

		username := msg.AuthorName
		if username == "" {
			username = m.getUsername(msg.AuthorID)
		} else {
			m.usernames[msg.AuthorID] = username
		}
		color := styles.UsernameColor(username)
		if msg.AuthorID == models.SparkBotID || strings.Contains(username, "🤖") {
			color = styles.Accent
		}

		if msg.AuthorID != lastAuthor {
			// New author header with user badge & timestamp
			if lastAuthor != 0 {
				b.WriteString("\n")
			}

			var authorBadge string
			if msg.AuthorID == models.SparkBotID || strings.Contains(username, "🤖") {
				authorBadge = lipgloss.NewStyle().
					Bold(true).
					Foreground(styles.Accent).
					Render("🤖 Spark")
			} else {
				authorBadge = lipgloss.NewStyle().
					Bold(true).
					Foreground(color).
					Render("● " + username)
			}

			timeStr := styles.MessageTime.Render(fmt.Sprintf("[%s]", timestamp))
			b.WriteString(fmt.Sprintf("  %s  %s\n", authorBadge, timeStr))
		}

		// Message content with colored left bar and word wrapping
		contentWidth := max(10, m.viewport.Width-8)
		wrappedContent := lipgloss.NewStyle().
			Width(contentWidth).
			Foreground(styles.TextBright).
			Render(msg.Content)

		bar := lipgloss.NewStyle().Foreground(color).Render("│")
		lines := strings.Split(wrappedContent, "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("    %s %s\n", bar, line))
		}

		lastAuthor = msg.AuthorID
	}

	// Typing indicator
	if len(m.typing) > 0 {
		b.WriteString("\n")
		typingText := strings.Join(m.typing, ", ")
		if len(m.typing) == 1 {
			if m.typing[0] == "Spark" {
				b.WriteString(styles.HelpStyle.Render("  💬 🤖 Spark is thinking..."))
			} else {
				typingText += " is typing..."
				b.WriteString(styles.HelpStyle.Render("  💬 " + typingText))
			}
		} else {
			typingText += " are typing..."
			b.WriteString(styles.HelpStyle.Render("  💬 " + typingText))
		}
	}

	return b.String()
}

func (m MessageView) getUsername(userID int64) string {
	if name, ok := m.usernames[userID]; ok {
		return name
	}
	return fmt.Sprintf("user_%d", userID)
}

func (m MessageView) formatTimestamp(t time.Time) string {
	loc := m.location
	if loc == nil {
		loc = time.Local
	}
	localTime := t.In(loc)
	now := time.Now().In(loc)
	if localTime.Day() == now.Day() && localTime.Month() == now.Month() && localTime.Year() == now.Year() {
		return localTime.Format("15:04")
	}
	return localTime.Format("02 Jan 15:04")
}
