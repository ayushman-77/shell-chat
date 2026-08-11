package actor

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ayushman-77/shell-chat/internal/models"
)

// --- Tea Messages that bridge Actor system -> BubbleTea ---

// ChatMsg is sent to the BubbleTea program when a new message is received.
type ChatMsg struct {
	Message *models.Message
}

// NewChatMsg creates a ChatMsg from a models.Message.
func NewChatMsg(msg *models.Message) ChatMsg {
	return ChatMsg{Message: msg}
}

// TypingMsg is sent to the BubbleTea program for typing indicators.
type TypingMsg struct {
	ChannelID int64
	UserID    int64
	Username  string
}

// SystemMsg is sent to the BubbleTea program for system notifications.
type SystemMsg struct {
	Content string
}

// MembersMsg is sent to the BubbleTea program when the online members list updates.
type MembersMsg struct {
	Users []OnlineUser
}

// ProfileUpdatedMsg is sent to the BubbleTea program when any user updates their profile.
type ProfileUpdatedMsg struct {
	UserID      int64
	OldUsername string
	NewUsername string
}

// SessionActor represents a single user's SSH session.
// It bridges the Actor system with the BubbleTea TUI program.
type SessionActor struct {
	mu        sync.RWMutex
	userID    int64
	username  string
	sessionID string
	guildID   int64        // currently viewed guild
	channelID int64        // currently viewed channel
	program   *tea.Program // reference to the TUI program for sending messages
	msgChan   chan tea.Msg // channel for BubbleTea command subscription loop
	registry  *Registry
}

// NewSessionActor creates a new session actor for a connected user.
func NewSessionActor(userID int64, username, sessionID string, program *tea.Program, registry *Registry) *SessionActor {
	return &SessionActor{
		userID:    userID,
		username:  username,
		sessionID: sessionID,
		program:   program,
		msgChan:   make(chan tea.Msg, 512),
		registry:  registry,
	}
}

// MsgChan returns the receive-only channel for BubbleTea updates.
func (s *SessionActor) MsgChan() <-chan tea.Msg {
	return s.msgChan
}

// Receive processes messages delivered to this session.
func (s *SessionActor) Receive(msg Message) {
	s.mu.RLock()
	prog := s.program
	s.mu.RUnlock()

	switch m := msg.(type) {
	case DeliverMessage:
		if prog != nil {
			prog.Send(NewChatMsg(m.Msg))
		}
		select {
		case s.msgChan <- NewChatMsg(m.Msg):
		default:
		}
	case DeliverTyping:
		if prog != nil {
			prog.Send(TypingMsg{
				ChannelID: m.ChannelID,
				UserID:    m.UserID,
				Username:  m.Username,
			})
		}
		select {
		case s.msgChan <- TypingMsg{
			ChannelID: m.ChannelID,
			UserID:    m.UserID,
			Username:  m.Username,
		}:
		default:
		}
	case SystemNotification:
		if prog != nil {
			prog.Send(SystemMsg{Content: m.Content})
		}
		select {
		case s.msgChan <- SystemMsg{Content: m.Content}:
		default:
		}
	case DeliverMembers:
		if prog != nil {
			prog.Send(MembersMsg{Users: m.Users})
		}
		select {
		case s.msgChan <- MembersMsg{Users: m.Users}:
		default:
		}
	case DeliverProfileUpdate:
		if prog != nil {
			prog.Send(ProfileUpdatedMsg{
				UserID:      m.UserID,
				OldUsername: m.OldUsername,
				NewUsername: m.NewUsername,
			})
		}
		select {
		case s.msgChan <- ProfileUpdatedMsg{
			UserID:      m.UserID,
			OldUsername: m.OldUsername,
			NewUsername: m.NewUsername,
		}:
		default:
		}
	}
}

// SetUsername updates the session's cached username.
func (s *SessionActor) SetUsername(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.username = username
}

// Stop performs cleanup when the session actor is being shut down.
func (s *SessionActor) Stop() {
	// Cleanup is handled by the SSH server when the connection closes
}

// SetProgram sets the BubbleTea program reference.
// Called after the TUI is initialized.
func (s *SessionActor) SetProgram(p *tea.Program) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.program = p
}

// SetActiveContext updates which guild and channel this session is currently viewing.
func (s *SessionActor) SetActiveContext(guildID, channelID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.guildID = guildID
	s.channelID = channelID
}

// SetActiveChannel updates which channel this session is currently viewing.
func (s *SessionActor) SetActiveChannel(channelID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.channelID = channelID
}

// ActiveChannel returns the channel this session is currently viewing.
func (s *SessionActor) ActiveChannel() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.channelID
}

// ActiveGuild returns the guild this session is currently viewing.
func (s *SessionActor) ActiveGuild() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.guildID
}

// SessionID returns the session ID.
func (s *SessionActor) SessionID() string {
	return s.sessionID
}

// UserID returns the user's ID.
func (s *SessionActor) UserID() int64 {
	return s.userID
}

// Username returns the user's username.
func (s *SessionActor) Username() string {
	return s.username
}
