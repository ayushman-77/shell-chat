package models

import "time"

// UserStatus represents the online status of a user.
type UserStatus int

const (
	StatusOffline UserStatus = iota
	StatusOnline
	StatusIdle
	StatusDND
)

const (
	// SparkBotID is the fixed virtual user ID for the Spark AI Bot.
	SparkBotID int64 = 999999999999999999
	// SparkBotName is the display name for Spark AI Bot.
	SparkBotName = "🤖 Spark"
)

// String returns the string representation of a UserStatus.
func (s UserStatus) String() string {
	switch s {
	case StatusOnline:
		return "online"
	case StatusIdle:
		return "idle"
	case StatusDND:
		return "dnd"
	default:
		return "offline"
	}
}

// User represents a registered user in the system.
type User struct {
	ID           int64
	Username     string
	DisplayName  string
	PasswordHash string
	Status       UserStatus
	CreatedAt    time.Time
}
