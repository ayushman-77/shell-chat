package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gocql/gocql"

	"github.com/ayushman-77/shell-chat/internal/models"
)

const (
	// DefaultCommunityGuildID is the global shared community guild for all connected SSH users.
	DefaultCommunityGuildID int64 = 100000000000000001
	// DefaultGeneralChannelID is the shared #general channel.
	DefaultGeneralChannelID int64 = 100000000000000002
	// DefaultDevChannelID is the shared #dev channel.
	DefaultDevChannelID int64 = 100000000000000003
	// DefaultRandomChannelID is the shared #random channel.
	DefaultRandomChannelID int64 = 100000000000000004
	// DefaultLoungeChannelID is the shared #lounge channel.
	DefaultLoungeChannelID int64 = 100000000000000005
	// DefaultAnnouncementsChannelID is the shared read-only #announcements channel.
	DefaultAnnouncementsChannelID int64 = 100000000000000006
)

// GuildStore handles guild and channel persistence in ScyllaDB with in-memory fallback.
type GuildStore struct {
	db         *DB
	mu         sync.RWMutex
	guilds     map[int64]*models.Guild
	userGuilds map[int64][]int64
	channels   map[int64][]*models.Channel
	members    map[int64][]*models.GuildMember
}

// NewGuildStore creates a new GuildStore.
func NewGuildStore(db *DB) *GuildStore {
	return &GuildStore{
		db:         db,
		guilds:     make(map[int64]*models.Guild),
		userGuilds: make(map[int64][]int64),
		channels:   make(map[int64][]*models.Channel),
		members:    make(map[int64][]*models.GuildMember),
	}
}

// EnsureDefaultCommunityGuild ensures the global community guild and channels exist,
// and enrolls the given user into the shared community guild.
func (s *GuildStore) EnsureDefaultCommunityGuild(ctx context.Context, userID int64) (*models.Guild, error) {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		g, exists := s.guilds[DefaultCommunityGuildID]
		if !exists {
			g = &models.Guild{
				ID:        DefaultCommunityGuildID,
				Name:      "Shell Chat Community",
				OwnerID:   0,
				CreatedAt: time.Now(),
			}
			s.guilds[DefaultCommunityGuildID] = g

			chAnnouncements := &models.Channel{
				ID:       DefaultAnnouncementsChannelID,
				GuildID:  DefaultCommunityGuildID,
				Name:     "announcements",
				Topic:    "Official community announcements and updates. (Read-only)",
				Type:     models.ChannelTypeAnnouncement,
				Position: 0,
			}
			chGeneral := &models.Channel{
				ID:       DefaultGeneralChannelID,
				GuildID:  DefaultCommunityGuildID,
				Name:     "general",
				Topic:    "Welcome to Shell Chat! Chat with everyone connected over SSH.",
				Type:     models.ChannelTypeText,
				Position: 1,
			}
			chDev := &models.Channel{
				ID:       DefaultDevChannelID,
				GuildID:  DefaultCommunityGuildID,
				Name:     "dev",
				Topic:    "Terminal UI, Distributed Systems, and SSH protocol discussions.",
				Type:     models.ChannelTypeText,
				Position: 2,
			}
			chRandom := &models.Channel{
				ID:       DefaultRandomChannelID,
				GuildID:  DefaultCommunityGuildID,
				Name:     "random",
				Topic:    "Casual chatter, off-topic memes, and fun discussions.",
				Type:     models.ChannelTypeText,
				Position: 3,
			}
			chLounge := &models.Channel{
				ID:       DefaultLoungeChannelID,
				GuildID:  DefaultCommunityGuildID,
				Name:     "lounge",
				Topic:    "Chill vibes, music, coffee, and hanging out.",
				Type:     models.ChannelTypeText,
				Position: 4,
			}
			s.channels[DefaultCommunityGuildID] = []*models.Channel{chAnnouncements, chGeneral, chDev, chRandom, chLounge}
		}

		if userID != 0 {
			alreadyMember := false
			for _, gid := range s.userGuilds[userID] {
				if gid == DefaultCommunityGuildID {
					alreadyMember = true
					break
				}
			}
			if !alreadyMember {
				s.userGuilds[userID] = append(s.userGuilds[userID], DefaultCommunityGuildID)
				s.members[DefaultCommunityGuildID] = append(s.members[DefaultCommunityGuildID], &models.GuildMember{
					GuildID:  DefaultCommunityGuildID,
					UserID:   userID,
					Role:     "member",
					JoinedAt: time.Now(),
				})
			}
		}

		gCopy := *g
		return &gCopy, nil
	}

	// ScyllaDB mode
	g := &models.Guild{
		ID:        DefaultCommunityGuildID,
		Name:      "Shell Chat Community",
		CreatedAt: time.Now(),
	}
	_ = s.CreateGuild(ctx, g)

	chAnnouncements := &models.Channel{
		ID:       DefaultAnnouncementsChannelID,
		GuildID:  DefaultCommunityGuildID,
		Name:     "announcements",
		Topic:    "Official community announcements and updates. (Read-only)",
		Type:     models.ChannelTypeAnnouncement,
		Position: 0,
	}
	_ = s.CreateChannel(ctx, chAnnouncements)

	chGeneral := &models.Channel{
		ID:       DefaultGeneralChannelID,
		GuildID:  DefaultCommunityGuildID,
		Name:     "general",
		Topic:    "Welcome to Shell Chat! Chat with everyone connected over SSH.",
		Type:     models.ChannelTypeText,
		Position: 1,
	}
	_ = s.CreateChannel(ctx, chGeneral)

	chDev := &models.Channel{
		ID:       DefaultDevChannelID,
		GuildID:  DefaultCommunityGuildID,
		Name:     "dev",
		Topic:    "Terminal UI, Distributed Systems, and SSH protocol discussions.",
		Type:     models.ChannelTypeText,
		Position: 2,
	}
	_ = s.CreateChannel(ctx, chDev)

	chRandom := &models.Channel{
		ID:       DefaultRandomChannelID,
		GuildID:  DefaultCommunityGuildID,
		Name:     "random",
		Topic:    "Casual chatter, off-topic memes, and fun discussions.",
		Type:     models.ChannelTypeText,
		Position: 3,
	}
	_ = s.CreateChannel(ctx, chRandom)

	chLounge := &models.Channel{
		ID:       DefaultLoungeChannelID,
		GuildID:  DefaultCommunityGuildID,
		Name:     "lounge",
		Topic:    "Chill vibes, music, coffee, and hanging out.",
		Type:     models.ChannelTypeText,
		Position: 4,
	}
	_ = s.CreateChannel(ctx, chLounge)

	if userID != 0 {
		_ = s.AddMember(ctx, &models.GuildMember{
			GuildID:  DefaultCommunityGuildID,
			UserID:   userID,
			Role:     "member",
			JoinedAt: time.Now(),
		})
	}

	return s.GetGuild(ctx, DefaultCommunityGuildID)
}

// CreateGuild creates a new guild.
func (s *GuildStore) CreateGuild(ctx context.Context, guild *models.Guild) error {
	if guild.CreatedAt.IsZero() {
		guild.CreatedAt = time.Now()
	}

	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		gCopy := *guild
		s.guilds[guild.ID] = &gCopy
		return nil
	}

	err := s.db.Session.Query(
		`INSERT INTO guilds (guild_id, name, owner_id, icon, created_at) VALUES (?, ?, ?, ?, ?)`,
		guild.ID, guild.Name, guild.OwnerID, guild.Icon, guild.CreatedAt,
	).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("create guild: %w", err)
	}
	return nil
}

// GetGuild retrieves a guild by ID.
func (s *GuildStore) GetGuild(ctx context.Context, guildID int64) (*models.Guild, error) {
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if g, ok := s.guilds[guildID]; ok {
			gCopy := *g
			return &gCopy, nil
		}
		return nil, fmt.Errorf("guild not found")
	}

	var guild models.Guild
	err := s.db.Session.Query(
		`SELECT guild_id, name, owner_id, icon, created_at FROM guilds WHERE guild_id = ?`,
		guildID,
	).WithContext(ctx).Scan(
		&guild.ID, &guild.Name, &guild.OwnerID, &guild.Icon, &guild.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get guild: %w", err)
	}
	return &guild, nil
}

// GetUserGuilds retrieves all guilds a user belongs to.
func (s *GuildStore) GetUserGuilds(ctx context.Context, userID int64) ([]*models.Guild, error) {
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		var guilds []*models.Guild
		for _, gID := range s.userGuilds[userID] {
			if g, ok := s.guilds[gID]; ok {
				gCopy := *g
				guilds = append(guilds, &gCopy)
			}
		}
		return guilds, nil
	}

	iter := s.db.Session.Query(
		`SELECT guild_id FROM user_guilds WHERE user_id = ?`,
		userID,
	).WithContext(ctx).Iter()

	var guildIDs []int64
	var guildID int64
	for iter.Scan(&guildID) {
		guildIDs = append(guildIDs, guildID)
	}
	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("get user guild ids: %w", err)
	}

	var guilds []*models.Guild
	for _, id := range guildIDs {
		guild, err := s.GetGuild(ctx, id)
		if err != nil {
			continue
		}
		guilds = append(guilds, guild)
	}

	return guilds, nil
}

// AddMember adds a user to a guild.
func (s *GuildStore) AddMember(ctx context.Context, member *models.GuildMember) error {
	if member.JoinedAt.IsZero() {
		member.JoinedAt = time.Now()
	}

	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		mCopy := *member
		s.members[member.GuildID] = append(s.members[member.GuildID], &mCopy)
		s.userGuilds[member.UserID] = append(s.userGuilds[member.UserID], member.GuildID)
		return nil
	}

	batch := s.db.Session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	batch.Query(
		`INSERT INTO guild_members (guild_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)`,
		member.GuildID, member.UserID, member.Role, member.JoinedAt,
	)

	batch.Query(
		`INSERT INTO user_guilds (user_id, guild_id) VALUES (?, ?)`,
		member.UserID, member.GuildID,
	)

	if err := s.db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

// RemoveMember removes a user from a guild.
func (s *GuildStore) RemoveMember(ctx context.Context, guildID, userID int64) error {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Remove from members
		var updated []*models.GuildMember
		for _, m := range s.members[guildID] {
			if m.UserID != userID {
				updated = append(updated, m)
			}
		}
		s.members[guildID] = updated

		// Remove from userGuilds
		var updatedGuilds []int64
		for _, gID := range s.userGuilds[userID] {
			if gID != guildID {
				updatedGuilds = append(updatedGuilds, gID)
			}
		}
		s.userGuilds[userID] = updatedGuilds
		return nil
	}

	batch := s.db.Session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	batch.Query(
		`DELETE FROM guild_members WHERE guild_id = ? AND user_id = ?`,
		guildID, userID,
	)

	batch.Query(
		`DELETE FROM user_guilds WHERE user_id = ? AND guild_id = ?`,
		userID, guildID,
	)

	if err := s.db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	return nil
}

// GetMembers retrieves all members of a guild.
func (s *GuildStore) GetMembers(ctx context.Context, guildID int64) ([]*models.GuildMember, error) {
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.members[guildID], nil
	}

	iter := s.db.Session.Query(
		`SELECT guild_id, user_id, role, joined_at FROM guild_members WHERE guild_id = ?`,
		guildID,
	).WithContext(ctx).Iter()

	var members []*models.GuildMember
	var m models.GuildMember
	for iter.Scan(&m.GuildID, &m.UserID, &m.Role, &m.JoinedAt) {
		member := m
		members = append(members, &member)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("get members: %w", err)
	}
	return members, nil
}

// CreateChannel creates a new channel in a guild.
func (s *GuildStore) CreateChannel(ctx context.Context, ch *models.Channel) error {
	if s.db == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		chCopy := *ch
		s.channels[ch.GuildID] = append(s.channels[ch.GuildID], &chCopy)
		return nil
	}

	err := s.db.Session.Query(
		`INSERT INTO channels (guild_id, channel_id, name, topic, type, position) VALUES (?, ?, ?, ?, ?, ?)`,
		ch.GuildID, ch.ID, ch.Name, ch.Topic, int(ch.Type), ch.Position,
	).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("create channel: %w", err)
	}
	return nil
}

// GetChannels retrieves all channels in a guild.
func (s *GuildStore) GetChannels(ctx context.Context, guildID int64) ([]*models.Channel, error) {
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.channels[guildID], nil
	}

	iter := s.db.Session.Query(
		`SELECT guild_id, channel_id, name, topic, type, position FROM channels WHERE guild_id = ?`,
		guildID,
	).WithContext(ctx).Iter()

	var channels []*models.Channel
	var ch models.Channel
	var chType int
	for iter.Scan(&ch.GuildID, &ch.ID, &ch.Name, &ch.Topic, &chType, &ch.Position) {
		channel := ch
		channel.Type = models.ChannelType(chType)
		channels = append(channels, &channel)
	}

	if err := iter.Close(); err != nil {
		return nil, fmt.Errorf("get channels: %w", err)
	}
	return channels, nil
}

// GetChannel retrieves a single channel.
func (s *GuildStore) GetChannel(ctx context.Context, guildID, channelID int64) (*models.Channel, error) {
	if s.db == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		for _, ch := range s.channels[guildID] {
			if ch.ID == channelID {
				chCopy := *ch
				return &chCopy, nil
			}
		}
		return nil, fmt.Errorf("channel not found")
	}

	var ch models.Channel
	var chType int

	err := s.db.Session.Query(
		`SELECT guild_id, channel_id, name, topic, type, position FROM channels WHERE guild_id = ? AND channel_id = ?`,
		guildID, channelID,
	).WithContext(ctx).Scan(
		&ch.GuildID, &ch.ID, &ch.Name, &ch.Topic, &chType, &ch.Position,
	)
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	ch.Type = models.ChannelType(chType)

	return &ch, nil
}
