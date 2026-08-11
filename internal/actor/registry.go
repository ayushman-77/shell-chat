package actor

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/log"

	"github.com/ayushman-77/shell-chat/internal/storage"
)

// OnlineUser represents an active online user in the system.
type OnlineUser struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
}

// Registry manages all active actors in the system.
type Registry struct {
	mu     sync.RWMutex
	actors map[string]*Ref
}

// NewRegistry creates a new actor registry.
func NewRegistry() *Registry {
	return &Registry{
		actors: make(map[string]*Ref),
	}
}

// GetOnlineUsers returns all unique, active logged-in users across all active sessions.
func (r *Registry) GetOnlineUsers() []OnlineUser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	seen := make(map[int64]bool)
	var users []OnlineUser

	for id, ref := range r.actors {
		if strings.HasPrefix(id, "session:") {
			if sess, ok := ref.actor.(*SessionActor); ok {
				uid := sess.UserID()
				uname := sess.Username()
				if uid != 0 && uname != "" && !seen[uid] {
					seen[uid] = true
					users = append(users, OnlineUser{
						UserID:   uid,
						Username: uname,
					})
				}
			}
		}
	}
	return users
}

// BroadcastOnlineUsers distributes the latest online members list to all active sessions.
func (r *Registry) BroadcastOnlineUsers() {
	users := r.GetOnlineUsers()
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, ref := range r.actors {
		if strings.HasPrefix(id, "session:") {
			ref.Send(DeliverMembers{Users: users})
		}
	}
}

// BroadcastProfileUpdate notifies all active sessions of a username change.
func (r *Registry) BroadcastProfileUpdate(userID int64, oldUsername, newUsername string) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for id, ref := range r.actors {
		if strings.HasPrefix(id, "session:") {
			if sess, ok := ref.actor.(*SessionActor); ok {
				if sess.UserID() == userID {
					sess.SetUsername(newUsername)
				}
			}
			ref.Send(DeliverProfileUpdate{
				UserID:      userID,
				OldUsername: oldUsername,
				NewUsername: newUsername,
			})
		}
	}
}

// Register adds an actor reference to the registry.
func (r *Registry) Register(ref *Ref) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actors[ref.id] = ref
}

// Unregister removes an actor from the registry by ID and closes it.
func (r *Registry) Unregister(id string) {
	r.mu.Lock()
	ref, ok := r.actors[id]
	if ok {
		delete(r.actors, id)
	}
	r.mu.Unlock()

	if ok && ref != nil {
		ref.Close()
	}

	if strings.HasPrefix(id, "session:") {
		r.BroadcastOnlineUsers()
	}
}

// Get retrieves an actor reference by ID.
func (r *Registry) Get(id string) (*Ref, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ref, ok := r.actors[id]
	return ref, ok
}

// GetOrCreateChannelActor retrieves an existing channel actor or creates a new one atomically.
func (r *Registry) GetOrCreateChannelActor(
	channelID, guildID int64,
	msgStore *storage.MessageStore,
	publisher EventPublisher,
	logger *log.Logger,
) *Ref {
	id := fmt.Sprintf("channel:%d", channelID)

	r.mu.Lock()
	defer r.mu.Unlock()

	if ref, ok := r.actors[id]; ok {
		return ref
	}

	actor := NewChannelActor(channelID, guildID, msgStore, publisher, r, logger)
	ref := NewRef(id, actor, 512, logger)
	r.actors[id] = ref
	return ref
}

// GetSessionsByUserID returns all active session actor references for the given user ID.
func (r *Registry) GetSessionsByUserID(userID int64) []*Ref {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var refs []*Ref
	for id, ref := range r.actors {
		if strings.HasPrefix(id, "session:") {
			if sess, ok := ref.actor.(*SessionActor); ok {
				if sess.UserID() == userID {
					refs = append(refs, ref)
				}
			}
		}
	}
	return refs
}

// GetByPrefix returns all actor references whose IDs start with the given prefix.
// Useful for finding all actors of a type (e.g., "session:" or "channel:").
func (r *Registry) GetByPrefix(prefix string) []*Ref {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var refs []*Ref
	for id, ref := range r.actors {
		if strings.HasPrefix(id, prefix) {
			refs = append(refs, ref)
		}
	}
	return refs
}

// Shutdown stops all registered actors and clears the registry.
func (r *Registry) Shutdown() {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, ref := range r.actors {
		ref.Close()
	}
	r.actors = make(map[string]*Ref)
}
