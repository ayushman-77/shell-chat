package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/log"

	"github.com/ayushman-77/shell-chat/config"
	"github.com/ayushman-77/shell-chat/internal/actor"
	"github.com/ayushman-77/shell-chat/internal/coalescer"
	"github.com/ayushman-77/shell-chat/internal/pubsub"
	"github.com/ayushman-77/shell-chat/internal/snowflake"
	sshserver "github.com/ayushman-77/shell-chat/internal/ssh"
	"github.com/ayushman-77/shell-chat/internal/storage"
)

func main() {
	// 1. Setup structured logger
	logger := log.NewWithOptions(os.Stderr, log.Options{
		Level:           log.InfoLevel,
		ReportTimestamp: true,
		Prefix:          "shell-chat",
	})

	logger.Info("Starting Shell Chat Server...")

	// 2. Load configuration
	cfg := config.Load()

	// 3. Initialize Snowflake ID generator
	if err := snowflake.Init(cfg.SnowflakeNodeID); err != nil {
		logger.Fatal("Failed to initialize snowflake", "err", err)
	}
	logger.Info("Snowflake ID generator initialized", "node_id", cfg.SnowflakeNodeID)

	// 4. Connect to ScyllaDB (with automatic in-memory fallback for local dev)
	var db *storage.DB
	var userStore *storage.UserStore
	var guildStore *storage.GuildStore
	var msgStore *storage.MessageStore

	db, err := storage.New(cfg.ScyllaHosts, cfg.ScyllaKeyspace)
	if err != nil {
		logger.Warn("ScyllaDB not available, running in-memory storage mode", "err", err)
		userStore = storage.NewUserStore(nil)
		guildStore = storage.NewGuildStore(nil)
		msgStore = storage.NewMessageStore(nil)

		// Seed a default public guild in in-memory mode
		seedDefaultGuild(guildStore, msgStore)
	} else {
		defer db.Close()
		logger.Info("Connected to ScyllaDB")

		if err := db.Migrate(); err != nil {
			logger.Fatal("Failed to run migrations", "err", err)
		}
		logger.Info("Database migrations completed")

		userStore = storage.NewUserStore(db)
		guildStore = storage.NewGuildStore(db)
		msgStore = storage.NewMessageStore(db)
	}

	// 5. Initialize Request Coalescer (Thundering Herd protection)
	coalescerInstance := coalescer.NewCoalescer(msgStore, guildStore, userStore)
	logger.Info("Request coalescer initialized")

	// 6. Connect to Redis (with automatic in-memory fallback)
	broker, err := pubsub.NewBroker(cfg.RedisAddr, logger)
	if err != nil {
		logger.Warn("Redis not available, running in-memory Pub/Sub broker", "err", err)
		broker = pubsub.NewMemoryBroker(logger)
	} else {
		logger.Info("Connected to Redis")
	}
	defer broker.Close()

	// 7. Initialize Actor registry
	registry := actor.NewRegistry()
	defer registry.Shutdown()
	logger.Info("Actor registry initialized")

	// 8. Start SSH server
	sshSrv, err := sshserver.NewServer(&sshserver.ServerConfig{
		Host:        cfg.SSHHost,
		Port:        cfg.SSHPort,
		HostKeyPath: cfg.HostKeyPath,
		Registry:    registry,
		Store:       db,
		Broker:      broker,
		Logger:      logger,
		UserStore:   userStore,
		GuildStore:  guildStore,
		MsgStore:    msgStore,
		Coalescer:    coalescerInstance,
		GeminiAPIKey: cfg.GeminiAPIKey,
		GeminiModel:  cfg.GeminiModel,
	})
	if err != nil {
		logger.Fatal("Failed to create SSH server", "err", err)
	}

	if err := sshSrv.Start(); err != nil {
		logger.Fatal("Failed to start SSH server", "err", err)
	}

	fmt.Println("")
	fmt.Println("  ╔══════════════════════════════════════════╗")
	fmt.Println("  ║       ⚡ SHELL CHAT SERVER RUNNING       ║")
	fmt.Println("  ╠══════════════════════════════════════════╣")
	fmt.Printf("  ║  Listening on %s:%-20s  ║\n", cfg.SSHHost, cfg.SSHPort)
	fmt.Println("  ╠══════════════════════════════════════════╣")
	fmt.Println("  ║  To connect:                             ║")
	fmt.Printf("  ║    ssh YOUR_IP -p %-20s  ║\n", cfg.SSHPort)
	fmt.Println("  ║                                          ║")
	fmt.Println("  ║  Press Ctrl+C to stop the server.        ║")
	fmt.Println("  ╚══════════════════════════════════════════╝")
	fmt.Println("")

	// 9. Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down...")

	// 10. Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := sshSrv.Shutdown(ctx); err != nil {
		logger.Error("SSH server shutdown error", "err", err)
	}

	logger.Info("Server stopped gracefully")
}

func seedDefaultGuild(guildStore *storage.GuildStore, msgStore *storage.MessageStore) {
	ctx := context.Background()
	_, _ = guildStore.EnsureDefaultCommunityGuild(ctx, 0)
}
