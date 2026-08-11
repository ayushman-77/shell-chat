package storage

import (
	"fmt"
	"time"

	"github.com/gocql/gocql"
)

// DB wraps a gocql session with helper methods for ScyllaDB operations.
type DB struct {
	Session *gocql.Session
}

// Schema statements for database migration.
var schemaStatements = []string{
	`CREATE KEYSPACE IF NOT EXISTS shell_chat WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`,
	`CREATE TABLE IF NOT EXISTS shell_chat.messages (
		channel_id bigint,
		bucket int,
		message_id bigint,
		author_id bigint,
		content text,
		created_at timestamp,
		PRIMARY KEY ((channel_id, bucket), message_id)
	) WITH CLUSTERING ORDER BY (message_id DESC)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.users (
		user_id bigint PRIMARY KEY,
		username text,
		display_name text,
		password_hash text,
		status int,
		created_at timestamp
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.users_by_username (
		username text PRIMARY KEY,
		user_id bigint
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.user_public_keys (
		fingerprint text PRIMARY KEY,
		user_id bigint,
		public_key_data text
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.guilds (
		guild_id bigint PRIMARY KEY,
		name text,
		owner_id bigint,
		icon text,
		created_at timestamp
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.guild_members (
		guild_id bigint,
		user_id bigint,
		role text,
		joined_at timestamp,
		PRIMARY KEY (guild_id, user_id)
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.channels (
		guild_id bigint,
		channel_id bigint,
		name text,
		topic text,
		type int,
		position int,
		PRIMARY KEY (guild_id, channel_id)
	)`,
	`CREATE TABLE IF NOT EXISTS shell_chat.user_guilds (
		user_id bigint,
		guild_id bigint,
		PRIMARY KEY (user_id, guild_id)
	)`,
}

// New creates a new DB connection to ScyllaDB.
func New(hosts []string, keyspace string) (*DB, error) {
	// First connect without keyspace to create it
	initCluster := gocql.NewCluster(hosts...)
	initCluster.Consistency = gocql.One
	initCluster.Timeout = 10 * time.Second
	initCluster.ConnectTimeout = 10 * time.Second

	initSession, err := initCluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("scylla connect (init): %w", err)
	}

	// Create keyspace
	if err := initSession.Query(schemaStatements[0]).Exec(); err != nil {
		initSession.Close()
		return nil, fmt.Errorf("create keyspace: %w", err)
	}
	initSession.Close()

	// Now connect with keyspace
	cluster := gocql.NewCluster(hosts...)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.LocalQuorum
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 10 * time.Second
	cluster.NumConns = 3

	session, err := cluster.CreateSession()
	if err != nil {
		return nil, fmt.Errorf("scylla connect: %w", err)
	}

	return &DB{Session: session}, nil
}

// Close closes the underlying ScyllaDB session.
func (db *DB) Close() {
	if db.Session != nil {
		db.Session.Close()
	}
}

// Migrate runs schema migrations — creates all tables if they don't exist.
func (db *DB) Migrate() error {
	// Skip the first statement (keyspace creation) since it was done in New()
	for i := 1; i < len(schemaStatements); i++ {
		if err := db.Session.Query(schemaStatements[i]).Exec(); err != nil {
			return fmt.Errorf("migration statement %d: %w", i, err)
		}
	}
	return nil
}
