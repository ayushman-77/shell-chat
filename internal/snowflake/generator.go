package snowflake

import (
	"fmt"
	"time"

	sf "github.com/bwmarrin/snowflake"
)

var node *sf.Node

// CustomEpoch is Jan 1, 2025 00:00:00 UTC in milliseconds.
var CustomEpoch int64 = 1735689600000

// Init initializes the Snowflake ID generator with the given node ID.
// Node IDs should be unique across all server instances (0-1023).
func Init(nodeID int64) error {
	sf.Epoch = CustomEpoch

	n, err := sf.NewNode(nodeID)
	if err != nil {
		return fmt.Errorf("snowflake init: %w", err)
	}
	node = n
	return nil
}

// Generate creates a new globally unique, time-sortable Snowflake ID.
func Generate() int64 {
	if node == nil {
		panic("snowflake: generator not initialized, call Init() first")
	}
	return node.Generate().Int64()
}

// ExtractTime extracts the timestamp from a Snowflake ID.
func ExtractTime(id int64) time.Time {
	timestampMs := (id >> 22) + CustomEpoch
	return time.UnixMilli(timestampMs)
}
