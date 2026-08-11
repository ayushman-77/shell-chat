package models

// ChannelType defines the kind of channel.
type ChannelType int

const (
	ChannelTypeText ChannelType = iota
	ChannelTypeAnnouncement
)

// String returns the string representation of a ChannelType.
func (t ChannelType) String() string {
	switch t {
	case ChannelTypeAnnouncement:
		return "announcement"
	default:
		return "text"
	}
}

// Channel represents a chat channel within a guild.
type Channel struct {
	ID       int64
	GuildID  int64
	Name     string
	Topic    string
	Type     ChannelType
	Position int
}

// DMChannelID generates a deterministic, unique 64-bit ID for a 1-on-1 direct message conversation.
func DMChannelID(user1, user2 int64) int64 {
	if user1 > user2 {
		user1, user2 = user2, user1
	}
	// FNV-1a 64-bit hash of composite user IDs
	h := uint64(14695981039346656037)
	str := "dm:" + string(rune(user1)) + ":" + string(rune(user2))
	for i := 0; i < len(str); i++ {
		h ^= uint64(str[i])
		h *= 1099511628211
	}
	// Combine with bitwise shift for uniqueness
	combined := uint64(user1)<<32 | uint64(user2&0xFFFFFFFF)
	h ^= combined
	return int64(h & 0x7FFFFFFFFFFFFFFF)
}
