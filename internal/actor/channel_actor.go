package actor

import (
	"context"

	"github.com/charmbracelet/log"

	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/storage"
)

// EventPublisher abstracts the Pub/Sub broker for cross-node distribution.
type EventPublisher interface {
	PublishMessage(ctx context.Context, guildID int64, msg *models.Message) error
	PublishTyping(ctx context.Context, guildID, channelID, userID int64, username string) error
	PublishPresence(ctx context.Context, guildID int64, userID int64, status models.UserStatus) error
}

// ChannelActor manages a single chat channel.
// It tracks connected members, handles message persistence, and routes messages.
type ChannelActor struct {
	channelID int64
	guildID   int64
	members   map[string]int64 // sessionID -> userID
	msgStore  *storage.MessageStore
	publisher EventPublisher
	registry  *Registry
	logger    *log.Logger
}

// NewChannelActor creates a new channel actor.
func NewChannelActor(
	channelID, guildID int64,
	msgStore *storage.MessageStore,
	publisher EventPublisher,
	registry *Registry,
	logger *log.Logger,
) *ChannelActor {
	return &ChannelActor{
		channelID: channelID,
		guildID:   guildID,
		members:   make(map[string]int64),
		msgStore:  msgStore,
		publisher: publisher,
		registry:  registry,
		logger:    logger,
	}
}

// Receive processes messages for this channel actor.
func (c *ChannelActor) Receive(msg Message) {
	switch m := msg.(type) {
	case PostMessage:
		ctx := context.Background()

		// 1. Record DM partner if this is a direct message
		if m.TargetUserID != 0 && c.msgStore != nil {
			c.msgStore.RecordDMPartner(m.Msg.AuthorID, m.TargetUserID)
		}

		// 2. Persist the message to ScyllaDB
		if c.msgStore != nil {
			if err := c.msgStore.SaveMessage(ctx, m.Msg); err != nil {
				if c.logger != nil {
					c.logger.Error("failed to save message", "channel", c.channelID, "err", err)
				}
			}
		}

		// 3. Publish to Redis Pub/Sub for cross-node fanout
		if c.publisher != nil {
			if err := c.publisher.PublishMessage(ctx, c.guildID, m.Msg); err != nil {
				if c.logger != nil {
					c.logger.Warn("failed to publish message to broker", "channel", c.channelID, "err", err)
				}
			}
		}

		// 4. Fan out to local sessions
		if m.TargetUserID != 0 && c.registry != nil {
			// 1-on-1 Direct Message: deliver to author and target user sessions
			delivered := make(map[string]bool)
			for _, ref := range c.registry.GetSessionsByUserID(m.Msg.AuthorID) {
				delivered[ref.id] = true
				ref.Send(DeliverMessage{Msg: m.Msg})
			}
			for _, ref := range c.registry.GetSessionsByUserID(m.TargetUserID) {
				if !delivered[ref.id] {
					delivered[ref.id] = true
					ref.Send(DeliverMessage{Msg: m.Msg})
				}
			}
		} else if c.registry != nil {
			// Guild / Server channel: deliver to ALL active sessions so real-time unread badges update everywhere!
			for _, ref := range c.registry.GetByPrefix("session:") {
				ref.Send(DeliverMessage{Msg: m.Msg})
			}
		}

	case JoinChannel:
		c.members[m.SessionID] = m.UserID
		if c.logger != nil {
			c.logger.Debug("user joined channel", "user", m.UserID, "channel", c.channelID, "session", m.SessionID)
		}

	case LeaveChannel:
		delete(c.members, m.SessionID)
		if c.logger != nil {
			c.logger.Debug("user left channel", "user", m.UserID, "channel", c.channelID, "session", m.SessionID)
		}

	case TypingStarted:
		ctx := context.Background()

		// 1. Publish to Redis Pub/Sub for other nodes
		if c.publisher != nil {
			_ = c.publisher.PublishTyping(ctx, c.guildID, m.ChannelID, m.UserID, m.Username)
		}

		// 2. Broadcast typing indicator to all local members except sender
		for sessionID, userID := range c.members {
			if userID != m.UserID {
				if ref, ok := c.registry.Get("session:" + sessionID); ok {
					ref.Send(DeliverTyping{
						ChannelID: m.ChannelID,
						UserID:    m.UserID,
						Username:  m.Username,
					})
				}
			}
		}
	}
}

// Stop performs cleanup when the channel actor is shut down.
func (c *ChannelActor) Stop() {
	if c.logger != nil {
		c.logger.Debug("channel actor stopped", "channel", c.channelID)
	}
}

// MemberCount returns the number of active members in this channel.
func (c *ChannelActor) MemberCount() int {
	return len(c.members)
}
