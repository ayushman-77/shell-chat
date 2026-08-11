package models

import "time"

// CustomEpoch is the epoch for Snowflake IDs: Jan 1, 2025 00:00:00 UTC (milliseconds).
const CustomEpoch int64 = 1735689600000

const (
	// BucketDuration is the duration of a single temporal bucket in milliseconds (1 week).
	BucketDuration int64 = 7 * 24 * 60 * 60 * 1000
)

// Message represents a chat message.
type Message struct {
	ID         int64     `json:"id"`
	ChannelID  int64     `json:"channel_id"`
	Bucket     int       `json:"bucket"`
	AuthorID   int64     `json:"author_id"`
	AuthorName string    `json:"author_name,omitempty"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
}

// BucketFromSnowflake extracts the temporal bucket from a Snowflake ID.
// The bucket groups messages by week for ScyllaDB partition distribution.
func BucketFromSnowflake(id int64) int {
	// Snowflake: top 41 bits are timestamp (ms since custom epoch)
	timestampMs := (id >> 22) + CustomEpoch
	return int((timestampMs - CustomEpoch) / BucketDuration)
}

// CurrentBucket returns the current temporal bucket number.
func CurrentBucket() int {
	now := time.Now().UnixMilli()
	return int((now - CustomEpoch) / BucketDuration)
}

// TimeFromSnowflake extracts the time from a Snowflake ID.
func TimeFromSnowflake(id int64) time.Time {
	timestampMs := (id >> 22) + CustomEpoch
	return time.UnixMilli(timestampMs)
}
