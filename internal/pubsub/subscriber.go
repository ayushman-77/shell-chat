package pubsub

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/charmbracelet/log"

	"github.com/ayushman-77/shell-chat/internal/actor"
	"github.com/ayushman-77/shell-chat/internal/models"
)

// SessionSubscriber manages Pub/Sub subscriptions for a single user session.
type SessionSubscriber struct {
	mu        sync.Mutex
	broker    *Broker
	sessionID string
	registry  *actor.Registry
	topics    map[string]context.CancelFunc
	logger    *log.Logger
}

// NewSessionSubscriber creates a subscriber for a session.
func NewSessionSubscriber(broker *Broker, sessionID string, registry *actor.Registry, logger *log.Logger) *SessionSubscriber {
	return &SessionSubscriber{
		broker:    broker,
		sessionID: sessionID,
		registry:  registry,
		topics:    make(map[string]context.CancelFunc),
		logger:    logger,
	}
}

// SubscribeChannel subscribes to a channel's message stream.
func (s *SessionSubscriber) SubscribeChannel(ctx context.Context, guildID, channelID int64) error {
	if s.broker == nil {
		return nil
	}

	topic := ChannelTopic(guildID, channelID)

	s.mu.Lock()
	// Unsubscribe from previous if exists
	if cancel, ok := s.topics[topic]; ok {
		cancel()
		delete(s.topics, topic)
	}

	subCtx, cancel := context.WithCancel(ctx)
	s.topics[topic] = cancel
	s.mu.Unlock()

	ch, err := s.broker.Subscribe(subCtx, topic)
	if err != nil {
		s.mu.Lock()
		cancel()
		delete(s.topics, topic)
		s.mu.Unlock()
		return err
	}

	// Process messages in background
	go func() {
		for data := range ch {
			s.handleEvent(data)
		}
	}()

	return nil
}

// UnsubscribeChannel unsubscribes from a channel.
func (s *SessionSubscriber) UnsubscribeChannel(guildID, channelID int64) {
	topic := ChannelTopic(guildID, channelID)
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, ok := s.topics[topic]; ok {
		cancel()
		delete(s.topics, topic)
	}
	if s.broker != nil {
		_ = s.broker.Unsubscribe(context.Background(), topic)
	}
}

// UnsubscribeAll unsubscribes from all topics.
func (s *SessionSubscriber) UnsubscribeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for topic, cancel := range s.topics {
		cancel()
		delete(s.topics, topic)
		if s.broker != nil {
			_ = s.broker.Unsubscribe(context.Background(), topic)
		}
	}
}

// handleEvent processes a raw Pub/Sub event.
func (s *SessionSubscriber) handleEvent(data []byte) {
	var event Event
	if err := json.Unmarshal(data, &event); err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to unmarshal event", "err", err)
		}
		return
	}

	ref, ok := s.registry.Get("session:" + s.sessionID)
	if !ok {
		return
	}

	switch event.Type {
	case EventNewMessage:
		var payload MessagePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		ref.Send(actor.DeliverMessage{
			Msg: &models.Message{
				ID:         payload.ID,
				ChannelID:  payload.ChannelID,
				Bucket:     models.BucketFromSnowflake(payload.ID),
				AuthorID:   payload.AuthorID,
				AuthorName: payload.AuthorName,
				Content:    payload.Content,
				CreatedAt:  time.UnixMilli(payload.Timestamp),
			},
		})

	case EventTyping:
		var payload TypingPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		ref.Send(actor.DeliverTyping{
			ChannelID: payload.ChannelID,
			UserID:    payload.UserID,
			Username:  payload.Username,
		})

	case EventPresence:
		var payload PresencePayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return
		}
		ref.Send(actor.UpdatePresence{
			UserID: payload.UserID,
			Status: models.UserStatus(payload.Status),
		})
	}
}
