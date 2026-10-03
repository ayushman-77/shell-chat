package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/segmentio/kafka-go"
)

// Broker manages Kafka connections for real-time message distribution,
// with automatic in-memory fallback for local development without Kafka.
type Broker struct {
	addr       string
	logger     *log.Logger
	writers    map[string]*kafka.Writer
	readers    map[string]*kafka.Reader
	memorySubs map[string][]chan []byte
	isMemory   bool
	mu         sync.RWMutex
}

// NewBroker creates a new Kafka broker.
func NewBroker(addr string, logger *log.Logger) (*Broker, error) {
	conn, err := kafka.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("kafka dial: %w", err)
	}
	conn.Close()

	return &Broker{
		addr:       addr,
		logger:     logger,
		writers:    make(map[string]*kafka.Writer),
		readers:    make(map[string]*kafka.Reader),
		memorySubs: make(map[string][]chan []byte),
		isMemory:   false,
	}, nil
}

// NewMemoryBroker creates an in-memory Pub/Sub broker for local standalone mode.
func NewMemoryBroker(logger *log.Logger) *Broker {
	return &Broker{
		logger:     logger,
		writers:    make(map[string]*kafka.Writer),
		readers:    make(map[string]*kafka.Reader),
		memorySubs: make(map[string][]chan []byte),
		isMemory:   true,
	}
}

// Close closes all subscriptions and the Kafka connections.
func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, w := range b.writers {
		w.Close()
	}
	for _, r := range b.readers {
		r.Close()
	}
	for _, chs := range b.memorySubs {
		for _, ch := range chs {
			close(ch)
		}
	}
	return nil
}

// ChannelTopic returns the Kafka topic name for a chat channel.
func ChannelTopic(guildID, channelID int64) string {
	return fmt.Sprintf("guild_%d_channel_%d", guildID, channelID)
}

// GuildTopic returns the Kafka topic name for guild-wide events.
func GuildTopic(guildID int64) string {
	return fmt.Sprintf("guild_%d_events", guildID)
}

// Publish publishes a message to a Kafka topic.
func (b *Broker) Publish(ctx context.Context, topic string, payload interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	if b.isMemory {
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

	b.mu.Lock()
	writer, exists := b.writers[topic]
	if !exists {
		writer = &kafka.Writer{
			Addr:                   kafka.TCP(b.addr),
			Topic:                  topic,
			Balancer:               &kafka.LeastBytes{},
			AllowAutoTopicCreation: true,
		}
		b.writers[topic] = writer
	}
	b.mu.Unlock()

	if err := writer.WriteMessages(ctx, kafka.Message{
		Value: data,
	}); err != nil {
		return err
	}

	return nil
}

// Subscribe subscribes to a Kafka topic and returns a channel of raw messages.
func (b *Broker) Subscribe(ctx context.Context, topic string) (<-chan []byte, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make(chan []byte, 256)
	b.memorySubs[topic] = append(b.memorySubs[topic], out)

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()

		chs := b.memorySubs[topic]
		for i, ch := range chs {
			if ch == out {
				b.memorySubs[topic] = append(chs[:i], chs[i+1:]...)
				break
			}
		}
		close(out)

		if len(b.memorySubs[topic]) == 0 && !b.isMemory {
			if r, ok := b.readers[topic]; ok {
				r.Close()
				delete(b.readers, topic)
			}
		}
	}()

	if b.isMemory {
		return out, nil
	}

	if _, exists := b.readers[topic]; !exists {
		reader := kafka.NewReader(kafka.ReaderConfig{
			Brokers: []string{b.addr},
			GroupID: fmt.Sprintf("gateway_%d", time.Now().UnixNano()),
			Topic:   topic,
		})
		b.readers[topic] = reader

		go func() {
			for {
				m, err := reader.ReadMessage(context.Background())
				if err != nil {
					return
				}
				
				b.mu.RLock()
				subs := b.memorySubs[topic]
				var chs []chan []byte
				for _, ch := range subs {
					chs = append(chs, ch)
				}
				b.mu.RUnlock()

				for _, ch := range chs {
					select {
					case ch <- m.Value:
					default:
					}
				}
			}
		}()
	}

	return out, nil
}

// Unsubscribe unsubscribes from a Kafka topic.
func (b *Broker) Unsubscribe(ctx context.Context, topic string) error {
	// Cleanup is handled entirely by the ctx passed to Subscribe
	return nil
}

// Ping checks Kafka connectivity.
func (b *Broker) Ping(ctx context.Context) error {
	if b.isMemory {
		return nil
	}
	conn, err := kafka.Dial("tcp", b.addr)
	if err != nil {
		return err
	}
	return conn.Close()
}
