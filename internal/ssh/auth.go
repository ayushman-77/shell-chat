package ssh

import (
	charmssh "github.com/charmbracelet/ssh"

	"github.com/ayushman-77/shell-chat/internal/storage"
)

// passwordAuthHandler returns a handler that accepts all password connections.
// Actual authentication is performed in the interactive TUI login view.
func passwordAuthHandler(users *storage.UserStore) func(ctx charmssh.Context, password string) bool {
	return func(ctx charmssh.Context, password string) bool {
		return true
	}
}

// publicKeyAuthHandler returns a handler that accepts all public keys.
// The user chooses/authenticates their account in the TUI.
func publicKeyAuthHandler(users *storage.UserStore) func(ctx charmssh.Context, key charmssh.PublicKey) bool {
	return func(ctx charmssh.Context, key charmssh.PublicKey) bool {
		return true
	}
}
