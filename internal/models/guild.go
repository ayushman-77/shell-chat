package models

import "time"

// Guild represents a chat server (similar to a Discord server).
type Guild struct {
	ID        int64
	Name      string
	OwnerID   int64
	Icon      string
	CreatedAt time.Time
}

// GuildMember represents a user's membership in a guild.
type GuildMember struct {
	GuildID  int64
	UserID   int64
	Role     string
	JoinedAt time.Time
}
