package pgstore

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type PGStore struct {
	DB *sqlx.DB
}

func New(dsn string) (*PGStore, error) {
	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &PGStore{DB: db}, nil
}

func (s *PGStore) Migrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		user_id BIGINT PRIMARY KEY,
		username VARCHAR(255) UNIQUE NOT NULL,
		display_name VARCHAR(255) NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		status INT NOT NULL DEFAULT 0,
		role VARCHAR(50) NOT NULL DEFAULT 'user',
		created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS user_public_keys (
		fingerprint VARCHAR(255) PRIMARY KEY,
		user_id BIGINT NOT NULL REFERENCES users(user_id) ON DELETE CASCADE,
		public_key_data TEXT NOT NULL
	);
	`

	_, err := s.DB.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("migrate users table: %w", err)
	}
	return nil
}
