package actor

import (
	"github.com/charmbracelet/log"
)

// Message is the interface for all actor messages.
type Message interface{}

// Actor is the interface that all actors must implement.
// An Actor processes messages sequentially — Receive is never called concurrently.
type Actor interface {
	// Receive processes a single message from the mailbox.
	Receive(msg Message)
	// Stop is called when the actor is being shut down for cleanup.
	Stop()
}

// Ref is a reference to a running actor, used to send messages to its mailbox.
type Ref struct {
	id      string
	mailbox chan Message
	actor   Actor
	done    chan struct{}
	logger  *log.Logger
}

// NewRef creates a new actor reference and starts its goroutine-based message loop.
// mailboxSize controls the buffered channel capacity.
func NewRef(id string, actor Actor, mailboxSize int, logger *log.Logger) *Ref {
	if mailboxSize <= 0 {
		mailboxSize = 256
	}

	ref := &Ref{
		id:      id,
		mailbox: make(chan Message, mailboxSize),
		actor:   actor,
		done:    make(chan struct{}),
		logger:  logger,
	}

	// Start the actor's processing goroutine — one goroutine per actor (green thread)
	go ref.run()

	return ref
}

// run is the actor's main loop. It processes messages sequentially from the mailbox.
func (r *Ref) run() {
	defer close(r.done)
	for msg := range r.mailbox {
		func() {
			defer func() {
				if p := recover(); p != nil {
					if r.logger != nil {
						r.logger.Error("actor panic recovered", "actor", r.id, "panic", p)
					}
				}
			}()
			r.actor.Receive(msg)
		}()
	}
	r.actor.Stop()
}

// Send enqueues a message to the actor's mailbox (non-blocking).
// If the mailbox is full, the message is dropped and a warning is logged.
func (r *Ref) Send(msg Message) {
	select {
	case r.mailbox <- msg:
	default:
		if r.logger != nil {
			r.logger.Warn("actor mailbox full, dropping message", "actor", r.id)
		}
	}
}

// ID returns the actor's unique identifier.
func (r *Ref) ID() string {
	return r.id
}

// Close shuts down the actor by closing its mailbox and waiting for it to drain.
func (r *Ref) Close() {
	close(r.mailbox)
	<-r.done
}
