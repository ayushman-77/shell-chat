package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	charmssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	lm "github.com/charmbracelet/wish/logging"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ayushman-77/shell-chat/internal/actor"
	"github.com/ayushman-77/shell-chat/internal/coalescer"
	"github.com/ayushman-77/shell-chat/internal/pubsub"
	"github.com/ayushman-77/shell-chat/internal/storage"
	"github.com/ayushman-77/shell-chat/internal/tui"
)

// ServerConfig holds configuration for the SSH server.
type ServerConfig struct {
	Host        string
	Port        string
	HostKeyPath string
	Registry    *actor.Registry
	Store       *storage.DB
	Broker      *pubsub.Broker
	Logger      *log.Logger
	UserStore   *storage.UserStore
	GuildStore  *storage.GuildStore
	MsgStore    *storage.MessageStore
	Coalescer    *coalescer.Coalescer
	GeminiAPIKey string
	GeminiModel  string
}

// Server holds the SSH server and its dependencies.
type Server struct {
	srv    *charmssh.Server
	logger *log.Logger
}

// NewServer creates a new SSH server with the full middleware chain.
func NewServer(cfg *ServerConfig) (*Server, error) {
	// Ensure host key directory exists
	if cfg.HostKeyPath != "" {
		dir := filepath.Dir(cfg.HostKeyPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("create host key directory: %w", err)
		}
	}

	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(cfg.Host, cfg.Port)),
		wish.WithHostKeyPath(cfg.HostKeyPath),
		// Accept all password auth — actual authentication happens in the TUI login view
		wish.WithPasswordAuth(passwordAuthHandler(cfg.UserStore)),
		wish.WithPublicKeyAuth(publicKeyAuthHandler(cfg.UserStore)),
		wish.WithMiddleware(
			// Middleware chain: FILO execution order
			// 1. BubbleTea middleware (outermost) — creates interactive TUI per session
			bm.Middleware(makeTeaHandler(cfg)),
			// 2. Active terminal middleware — reject non-PTY connections
			activeterm.Middleware(),
			// 3. Security middleware — drop malicious envs, reject port forwarding
			SecurityMiddleware(),
			// 4. Logging middleware
			lm.Middleware(),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create ssh server: %w", err)
	}

	return &Server{srv: srv, logger: cfg.Logger}, nil
}

// makeTeaHandler creates the BubbleTea handler function that runs per SSH session.
func makeTeaHandler(cfg *ServerConfig) bm.Handler {
	return func(s charmssh.Session) (tea.Model, []tea.ProgramOption) {
		pty, _, active := s.Pty()
		if !active {
			wish.Fatalln(s, "terminal is not interactive")
			return nil, nil
		}

		app := tui.NewApp(
			s.User(),
			pty.Window.Width,
			pty.Window.Height,
			cfg.Registry,
			cfg.UserStore,
			cfg.GuildStore,
			cfg.MsgStore,
			cfg.Coalescer,
			cfg.Broker,
			cfg.Logger,
		)
		app.SetAIConfig(cfg.GeminiAPIKey, cfg.GeminiModel)

		// Auto-cleanup session when client disconnects
		go func() {
			<-s.Context().Done()
			if cfg.Registry != nil {
				cfg.Registry.Unregister("session:" + app.SessionID())
			}
		}()

		return app, []tea.ProgramOption{tea.WithAltScreen()}
	}
}

// Start starts the SSH server in a background goroutine.
func (s *Server) Start() error {
	go func() {
		if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, charmssh.ErrServerClosed) {
			s.logger.Fatal("SSH server error", "err", err)
		}
	}()
	return nil
}

// Shutdown gracefully shuts down the SSH server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
