package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/charmbracelet/log"
	"github.com/redis/go-redis/v9"
)

// Broker manages Redis Pub/Sub connections for real-time message distribution,
// with automatic in-memory fallback for local development without Redis.
type Broker struct {
	client     *redis.Client
	logger     *log.Logger
	subs       map[string]*redis.PubSub
	memorySubs map[string][]chan []byte
	mu         sync.RWMutex
}

// NewBroker creates a new Redis Pub/Sub broker.
func NewBroker(addr string, logger *log.Logger) (*Broker, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		PoolSize: 10,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	return &Broker{
		client:     client,
		logger:     logger,
		subs:       make(map[string]*redis.PubSub),
		memorySubs: make(map[string][]chan []byte),
	}, nil
}

// NewMemoryBroker creates an in-memory Pub/Sub broker for local standalone mode.
func NewMemoryBroker(logger *log.Logger) *Broker {
	return &Broker{
		logger:     logger,
		subs:       make(map[string]*redis.PubSub),
		memorySubs: make(map[string][]chan []byte),
	}
}

// Close closes all subscriptions and the Redis client.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for topic, sub := range b.subs {
		if err := sub.Close(); err != nil && b.logger != nil {
			b.logger.Warn("error closing subscription", "topic", topic, "err", err)
		}
	}
	b.subs = make(map[string]*redis.PubSub)

	for topic, chs := range b.memorySubs {
		for _, ch := range chs {
			close(ch)
		}
		delete(b.memorySubs, topic)
	}

	if b.client != nil {
		return b.client.Close()
	}
	return nil
}

// ChannelTopic returns the Pub/Sub topic name for a chat channel.
func ChannelTopic(guildID, channelID int64) string {
	return fmt.Sprintf("guild:%d:channel:%d:stream", guildID, channelID)
}

// GuildTopic returns the Pub/Sub topic name for guild-wide events.
func GuildTopic(guildID int64) string {
	return fmt.Sprintf("guild:%d:events", guildID)
}

// Publish publishes a message to a Pub/Sub topic.
func (b *Broker) Publish(ctx context.Context, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	// In-memory distribution
	if b.client == nil {
		b.mu.RLock()
		defer b.mu.RUnlock()
		if chs, ok := b.memorySubs[topic]; ok {
			for _, ch := range chs {
				select {
				case ch <- data:
				default:
				}
			}
		}
		return nil
	}

	if err := b.client.Publish(ctx, topic, data).Err(); err != nil {
		return fmt.Errorf("publish to %s: %w", topic, err)
	}

	return nil
}

// Subscribe subscribes to a Pub/Sub topic and returns a channel of raw messages.
func (b *Broker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make(chan []byte, 256)

	// In-memory subscription
	if b.client == nil {
		b.memorySubs[topic] = append(b.memorySubs[topic], out)
		return out, nil
	}

	pubsub := b.client.Subscribe(ctx, topic)

	// Verify subscription
	if _, err := pubsub.Receive(ctx); err != nil {
		pubsub.Close()
		return nil, fmt.Errorf("subscribe to %s: %w", topic, err)
	}

	b.subs[topic] = pubsub

	go func() {
		defer close(out)
		ch := pubsub.Channel()
		for msg := range ch {
			select {
			case out <- []byte(msg.Payload):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

// Unsubscribe unsubscribes from a Pub/Sub topic.
func (b *Broker) Unsubscribe(ctx context.Context, topic string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.client == nil {
		if chs, ok := b.memorySubs[topic]; ok {
			for _, ch := range chs {
				close(ch)
			}
			delete(b.memorySubs, topic)
		}
		return nil
	}

	if sub, ok := b.subs[topic]; ok {
		err := sub.Close()
		delete(b.subs, topic)
		return err
	}
	return nil
}

// Ping checks Redis connectivity.
func (b *Broker) Ping(ctx context.Context) error {
	if b.client == nil {
		return nil
	}
	return b.client.Ping(ctx).Err()
}
