package actor

import (
	"github.com/ayushman-77/shell-chat/internal/models"
)

// --- Chat Messages ---

// PostMessage is sent to a ChannelActor when a user sends a chat message.
type PostMessage struct {
	Msg          *models.Message
	TargetUserID int64
}

// MessagePosted is an acknowledgment that a message was persisted.
type MessagePosted struct {
	Msg *models.Message
}

// --- Channel Lifecycle ---

// JoinChannel is sent to a ChannelActor when a user navigates to it.
type JoinChannel struct {
	UserID    int64
	SessionID string
}

// LeaveChannel is sent to a ChannelActor when a user navigates away.
type LeaveChannel struct {
	UserID    int64
	SessionID string
}

// --- Presence ---

// UpdatePresence is sent when a user's online status changes.
type UpdatePresence struct {
	UserID int64
	Status models.UserStatus
}

// TypingStarted is sent when a user begins typing in a channel.
type TypingStarted struct {
	ChannelID int64
	UserID    int64
	Username  string
}

// --- System ---

// SystemNotification is a server-generated notification.
type SystemNotification struct {
	Content string
}

// --- Session-specific messages (delivered to SessionActors) ---

// DeliverMessage is sent to a SessionActor to render a new chat message.
type DeliverMessage struct {
	Msg *models.Message
}

// DeliverTyping is sent to a SessionActor to show a typing indicator.
type DeliverTyping struct {
	ChannelID int64
	UserID    int64
	Username  string
}

// DeliverMembers is sent to a SessionActor to update the list of online users.
type DeliverMembers struct {
	Users []OnlineUser
}

// DeliverProfileUpdate is sent to SessionActors when a user updates their username.
type DeliverProfileUpdate struct {
	UserID      int64
	OldUsername string
	NewUsername string
}
