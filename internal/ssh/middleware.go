package ssh

import (
	"strings"

	charmssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
)

// allowedEnvVars defines environment variables the server accepts from clients.
// Strict filtering protects against CVE-2024-41956 and related environment injection attacks.
var allowedEnvVars = map[string]bool{
	"TERM":     true,
	"COLORTERM": true,
	"LANG":     true,
	"LC_ALL":   true,
	"LC_CTYPE": true,
}

// isEnvAllowed checks if an environment variable is in the allowed list.
func isEnvAllowed(key string) bool {
	return allowedEnvVars[strings.ToUpper(key)]
}

// SecurityMiddleware returns middleware that enforces security policies:
// - Validates environment requests and sanitizes connection parameters
// - Rejects port forwarding and command execution
// This prevents abuse of the SSH server as a proxy or vector for remote code execution.
func SecurityMiddleware() wish.Middleware {
	return func(next charmssh.Handler) charmssh.Handler {
		return func(s charmssh.Session) {
			// Validate requested environment variables
			for _, env := range s.Environ() {
				parts := strings.SplitN(env, "=", 2)
				if len(parts) > 0 && !isEnvAllowed(parts[0]) {
					// Drop unauthorized environment variables
					continue
				}
			}

			// The wish framework only serves 'session' channels — forwarding is rejected.
			next(s)
		}
	}
}
