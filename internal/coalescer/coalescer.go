package coalescer

import (
	"context"
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/singleflight"

	"github.com/ayushman-77/shell-chat/internal/models"
	"github.com/ayushman-77/shell-chat/internal/storage"
)

// Stats tracks coalescing metrics.
type Stats struct {
	TotalRequests    uint64
	CoalescedShared  uint64
	ExecutedQueries  uint64
}

// Coalescer wraps storage operations with request coalescing
// using singleflight to eliminate thundering herds on hot partitions.
type Coalescer struct {
	group      singleflight.Group
	msgStore   *storage.MessageStore
	guildStore *storage.GuildStore
	userStore  *storage.UserStore

	totalRequests   uint64
	coalescedShared uint64
	executedQueries uint64
}

// NewCoalescer creates a new request coalescer.
func NewCoalescer(
	msgStore *storage.MessageStore,
	guildStore *storage.GuildStore,
	userStore *storage.UserStore,
) *Coalescer {
	return &Coalescer{
		msgStore:   msgStore,
		guildStore: guildStore,
		userStore:  userStore,
	}
}

// GetMessages coalesces identical message fetch requests.
// When thousands of users open the same channel simultaneously,
// singleflight collapses all redundant requests into a single database read.
func (c *Coalescer) GetMessages(ctx context.Context, channelID int64, before int64, limit int) ([]*models.Message, error) {
	atomic.AddUint64(&c.totalRequests, 1)
	key := fmt.Sprintf("messages:%d:%d:%d", channelID, before, limit)

	result, err, shared := c.group.Do(key, func() (interface{}, error) {
		atomic.AddUint64(&c.executedQueries, 1)
		if c.msgStore == nil {
			return []*models.Message{}, nil
		}
		return c.msgStore.GetMessages(ctx, channelID, before, limit)
	})

	if shared {
		atomic.AddUint64(&c.coalescedShared, 1)
	}

	if err != nil {
		return nil, err
	}
	return result.([]*models.Message), nil
}

// GetGuildChannels coalesces channel list requests for a guild.
func (c *Coalescer) GetGuildChannels(ctx context.Context, guildID int64) ([]*models.Channel, error) {
	atomic.AddUint64(&c.totalRequests, 1)
	key := fmt.Sprintf("channels:%d", guildID)

	result, err, shared := c.group.Do(key, func() (interface{}, error) {
		atomic.AddUint64(&c.executedQueries, 1)
		if c.guildStore == nil {
			return []*models.Channel{}, nil
		}
		return c.guildStore.GetChannels(ctx, guildID)
	})

	if shared {
		atomic.AddUint64(&c.coalescedShared, 1)
	}

	if err != nil {
		return nil, err
	}
	return result.([]*models.Channel), nil
}

// GetUserGuilds coalesces guild list requests for a user.
func (c *Coalescer) GetUserGuilds(ctx context.Context, userID int64) ([]*models.Guild, error) {
	atomic.AddUint64(&c.totalRequests, 1)
	key := fmt.Sprintf("user_guilds:%d", userID)

	result, err, shared := c.group.Do(key, func() (interface{}, error) {
		atomic.AddUint64(&c.executedQueries, 1)
		if c.guildStore == nil {
			return []*models.Guild{}, nil
		}
		return c.guildStore.GetUserGuilds(ctx, userID)
	})

	if shared {
		atomic.AddUint64(&c.coalescedShared, 1)
	}

	if err != nil {
		return nil, err
	}
	return result.([]*models.Guild), nil
}

// GetUserByID coalesces user profile lookups.
func (c *Coalescer) GetUserByID(ctx context.Context, userID int64) (*models.User, error) {
	atomic.AddUint64(&c.totalRequests, 1)
	key := fmt.Sprintf("user:%d", userID)

	result, err, shared := c.group.Do(key, func() (interface{}, error) {
		atomic.AddUint64(&c.executedQueries, 1)
		if c.userStore == nil {
			return nil, fmt.Errorf("user store not configured")
		}
		return c.userStore.GetUserByID(ctx, userID)
	})

	if shared {
		atomic.AddUint64(&c.coalescedShared, 1)
	}

	if err != nil {
		return nil, err
	}
	return result.(*models.User), nil
}

// GetStats returns current coalescing statistics.
func (c *Coalescer) GetStats() Stats {
	return Stats{
		TotalRequests:   atomic.LoadUint64(&c.totalRequests),
		CoalescedShared: atomic.LoadUint64(&c.coalescedShared),
		ExecutedQueries: atomic.LoadUint64(&c.executedQueries),
	}
}
