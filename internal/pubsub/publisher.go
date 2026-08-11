package pubsub

import (
	"context"
	"encoding/json"

	"github.com/ayushman-77/shell-chat/internal/models"
)

// Event types for the Pub/Sub system.
const (
	EventNewMessage  = "new_message"
	EventTyping      = "typing"
	EventPresence    = "presence"
	EventMemberJoin  = "member_join"
	EventMemberLeave = "member_leave"
)

// Event is the envelope for Pub/Sub messages.
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// MessagePayload is the payload for new message events.
type MessagePayload struct {
	ID         int64  `json:"id"`
	ChannelID  int64  `json:"channel_id"`
	AuthorID   int64  `json:"author_id"`
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
	Timestamp  int64  `json:"timestamp"`
}

// TypingPayload is the payload for typing indicator events.
type TypingPayload struct {
	ChannelID int64  `json:"channel_id"`
	UserID    int64  `json:"user_id"`
	Username  string `json:"username"`
}

// PresencePayload is the payload for presence update events.
type PresencePayload struct {
	UserID int64 `json:"user_id"`
	Status int   `json:"status"`
}

// PublishMessage publishes a new message event to the channel's topic.
func (b *Broker) PublishMessage(ctx context.Context, guildID int64, msg *models.Message) error {
	raw, err := json.Marshal(MessagePayload{
		ID:         msg.ID,
		ChannelID:  msg.ChannelID,
		AuthorID:   msg.AuthorID,
		AuthorName: msg.AuthorName,
		Content:    msg.Content,
		Timestamp:  msg.CreatedAt.UnixMilli(),
	})
	if err != nil {
		return err
	}
	event := Event{
		Type:    EventNewMessage,
		Payload: raw,
	}
	return b.Publish(ctx, ChannelTopic(guildID, msg.ChannelID), event)
}

// PublishTyping publishes a typing indicator to the channel's topic.
func (b *Broker) PublishTyping(ctx context.Context, guildID, channelID, userID int64, username string) error {
	raw, _ := json.Marshal(TypingPayload{
		ChannelID: channelID,
		UserID:    userID,
		Username:  username,
	})
	event := Event{
		Type:    EventTyping,
		Payload: raw,
	}
	return b.Publish(ctx, ChannelTopic(guildID, channelID), event)
}

// PublishPresence publishes a presence update to the guild's topic.
func (b *Broker) PublishPresence(ctx context.Context, guildID int64, userID int64, status models.UserStatus) error {
	raw, _ := json.Marshal(PresencePayload{
		UserID: userID,
		Status: int(status),
	})
	event := Event{
		Type:    EventPresence,
		Payload: raw,
	}
	return b.Publish(ctx, GuildTopic(guildID), event)
}
