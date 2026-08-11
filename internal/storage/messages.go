package storage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ayushman-77/shell-chat/internal/models"
)

// MessageStore handles message persistence in ScyllaDB with in-memory fallback.
type MessageStore struct {
	db       *DB
	mu       sync.RWMutex
	messages map[int64][]*models.Message // channelID -> messages (stored DESC order)
	userDMs  map[int64]map[int64]bool    // userID -> set of partner userIDs
}

// NewMessageStore creates a new MessageStore.
func NewMessageStore(db *DB) *MessageStore {
	return &MessageStore{
		db:       db,
		messages: make(map[int64][]*models.Message),
		userDMs:  make(map[int64]map[int64]bool),
	}
}

// RecordDMPartner registers a DM conversation between user1 and user2.
func (s *MessageStore) RecordDMPartner(user1, user2 int64) {
	if user1 == 0 || user2 == 0 || user1 == user2 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.userDMs == nil {
		s.userDMs = make(map[int64]map[int64]bool)
	}
	if s.userDMs[user1] == nil {
		s.userDMs[user1] = make(map[int64]bool)
	}
	if s.userDMs[user2] == nil {
		s.userDMs[user2] = make(map[int64]bool)
	}
	s.userDMs[user1][user2] = true
	s.userDMs[user2][user1] = true
}

// GetDMPartners returns all partner user IDs that userID has had a DM conversation with.
func (s *MessageStore) GetDMPartners(userID int64) []int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.userDMs == nil || s.userDMs[userID] == nil {
		return nil
	}
	var partners []int64
	for p := range s.userDMs[userID] {
		partners = append(partners, p)
	}
	return partners
}

// SaveMessage persists a message to ScyllaDB with temporal bucketing (or memory).
func (s *MessageStore) SaveMessage(ctx context.Context, msg *models.Message) error {
	if msg.Bucket == 0 {
		msg.Bucket = models.BucketFromSnowflake(msg.ID)
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = models.TimeFromSnowflake(msg.ID)
	}

	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Deduplicate if already saved
		for _, existing := range s.messages[msg.ChannelID] {
			if existing.ID != 0 && existing.ID == msg.ID {
				return nil
			}
		}
		msgCopy := *msg
		// Prepend to maintain DESC order (newest first)
		s.messages[msg.ChannelID] = append([]*models.Message{&msgCopy}, s.messages[msg.ChannelID]...)
		return nil
	}

	query := `INSERT INTO messages (channel_id, bucket, message_id, author_id, content, created_at) 
			  VALUES (?, ?, ?, ?, ?, ?)`

	err := s.db.Session.Query(query,
		msg.ChannelID,
		msg.Bucket,
		msg.ID,
		msg.AuthorID,
		msg.Content,
		msg.CreatedAt,
	).WithContext(ctx).Exec()

	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

// GetMessages retrieves messages for a channel with cursor-based pagination.
// If before is 0, gets the latest messages from the current bucket.
// If before > 0, gets messages older than the given Snowflake ID.
func (s *MessageStore) GetMessages(ctx context.Context, channelID int64, before int64, limit int) ([]*models.Message, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		var res []*models.Message
		for _, m := range s.messages[channelID] {
			if before > 0 && m.ID >= before {
				continue
			}
			mCopy := *m
			res = append(res, &mCopy)
			if len(res) >= limit {
				break
			}
		}
		return res, nil
	}

	var messages []*models.Message
	var startBucket int

	if before > 0 {
		startBucket = models.BucketFromSnowflake(before)
	} else {
		startBucket = models.CurrentBucket()
	}

	// Try up to 3 buckets to fill the requested limit
	for bucket := startBucket; bucket >= 0 && len(messages) < limit; bucket-- {
		remaining := limit - len(messages)
		bucketMsgs, err := s.getMessagesFromBucket(ctx, channelID, bucket, before, remaining)
		if err != nil {
			return nil, err
		}
		messages = append(messages, bucketMsgs...)

		// If we got messages from this bucket, don't use `before` cursor for previous buckets
		if len(bucketMsgs) > 0 {
			before = 0
		}

		// Don't go back more than 3 empty buckets
		if bucket < startBucket-3 && len(messages) == 0 {
			break
		}
	}

	return messages, nil
}

func (s *MessageStore) getMessagesFromBucket(ctx context.Context, channelID int64, bucket int, before int64, limit int) ([]*models.Message, error) {
	var query string
	var args []interface{}

	if before > 0 {
		query = `SELECT message_id, channel_id, author_id, content, created_at 
				 FROM messages 
				 WHERE channel_id = ? AND bucket = ? AND message_id < ? 
				 LIMIT ?`
		args = []interface{}{channelID, bucket, before, limit}
	} else {
		query = `SELECT message_id, channel_id, author_id, content, created_at 
				 FROM messages 
				 WHERE channel_id = ? AND bucket = ? 
				 LIMIT ?`
		args = []interface{}{channelID, bucket, limit}
	}

	iter := s.db.Session.Query(query, args...).WithContext(ctx).Iter()

	var messages []*models.Message
	var msgID, authorID int64
	var content string
	var createdAt time.Time

	for iter.Scan(&msgID, &channelID, &authorID, &content, &createdAt) {
		messages = append(messages, &models.Message{
			ID:        msgID,
			ChannelID: channelID,
			Bucket:    bucket,
			AuthorID:  authorID,
			Content:   content,
			CreatedAt: createdAt,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("get messages bucket %d: %w", bucket, err)
	}

	return messages, nil
}

// SearchMessages searches for messages containing query (case-insensitive) in the given channel.
// If channelID is 0, it searches across all channels.
func (s *MessageStore) SearchMessages(ctx context.Context, channelID int64, query string, limit int) []*models.Message {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var matches []*models.Message

	searchList := func(list []*models.Message) {
		for _, m := range list {
			if len(matches) >= limit {
				return
			}
			if strings.Contains(strings.ToLower(m.Content), queryLower) {
				mCopy := *m
				matches = append(matches, &mCopy)
			}
		}
	}

	if channelID != 0 {
		searchList(s.messages[channelID])
	} else {
		for _, list := range s.messages {
			searchList(list)
			if len(matches) >= limit {
				break
			}
		}
	}

	return matches
}

// UpdateAuthorName retroactively updates the author name on all messages written by authorID across all channels.
func (s *MessageStore) UpdateAuthorName(ctx context.Context, authorID int64, newAuthorName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, msgList := range s.messages {
		for _, m := range msgList {
			if m.AuthorID == authorID {
				m.AuthorName = newAuthorName
			}
		}
	}

	if s.db != nil {
		// In ScyllaDB mode, update messages in database if table stores author_name
		_ = s.db.Session.Query(`UPDATE messages SET author_name = ? WHERE author_id = ?`, newAuthorName, authorID).WithContext(ctx).Exec()
	}

	return nil
}
