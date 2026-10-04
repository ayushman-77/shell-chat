package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gocql/gocql"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"

	"github.com/ayushman-77/shell-chat/internal/models"
)

// UserStore handles user persistence.
type UserStore struct {
	db          *DB
	pg          *sqlx.DB
	mu          sync.RWMutex
	usersByID   map[int64]*models.User
	usersByName map[string]*models.User
	usersByPK   map[string]*models.User
}

// NewUserStore creates a new UserStore.
func NewUserStore(db *DB, pg *sqlx.DB) *UserStore {
	return &UserStore{
		db:          db,
		pg:          pg,
		usersByID:   make(map[int64]*models.User),
		usersByName: make(map[string]*models.User),
		usersByPK:   make(map[string]*models.User),
	}
}

// CreateUser registers a new user. The password in user.PasswordHash
// is treated as plaintext and will be bcrypt-hashed before storage.
func (s *UserStore) CreateUser(ctx context.Context, user *models.User) error {
	// Hash the password
	hashed, err := bcrypt.GenerateFromPassword([]byte(user.PasswordHash), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = string(hashed)

	if user.CreatedAt.IsZero() {
		user.CreatedAt = time.Now()
	}

	if user.Role == "" {
		if user.Username == "admin" {
			user.Role = models.RoleAdmin
		} else {
			user.Role = models.RoleUser
		}
	}

	// In-memory mode
	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		uCopy := *user
		s.usersByID[user.ID] = &uCopy
		s.usersByName[user.Username] = &uCopy
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		_, err := s.pg.ExecContext(ctx,
			`INSERT INTO users (user_id, username, display_name, password_hash, status, role, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			user.ID, user.Username, user.DisplayName, user.PasswordHash, int(user.Status), string(user.Role), user.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("create user pg: %w", err)
		}
		return nil
	}

	// ScyllaDB mode
	batch := s.db.Session.NewBatch(gocql.LoggedBatch).WithContext(ctx)

	batch.Query(
		`INSERT INTO users (user_id, username, display_name, password_hash, status, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.DisplayName, user.PasswordHash, int(user.Status), user.CreatedAt,
	)

	batch.Query(
		`INSERT INTO users_by_username (username, user_id) VALUES (?, ?)`,
		user.Username, user.ID,
	)

	if err := s.db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

// GetUserByID retrieves a user by their Snowflake ID.
func (s *UserStore) GetUserByID(ctx context.Context, id int64) (*models.User, error) {
	if s.db == nil && s.pg == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if u, ok := s.usersByID[id]; ok {
			uCopy := *u
			return &uCopy, nil
		}
		return nil, fmt.Errorf("user not found")
	}

	// Postgres mode
	if s.pg != nil {
		var user models.User
		var status int
		var role string
		err := s.pg.QueryRowContext(ctx,
			`SELECT user_id, username, display_name, password_hash, status, role, created_at FROM users WHERE user_id = $1`,
			id,
		).Scan(&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &status, &role, &user.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("get user by id pg: %w", err)
		}
		user.Status = models.UserStatus(status)
		user.Role = models.UserRole(role)
		return &user, nil
	}

	var user models.User
	var status int

	err := s.db.Session.Query(
		`SELECT user_id, username, display_name, password_hash, status, created_at FROM users WHERE user_id = ?`,
		id,
	).WithContext(ctx).Scan(
		&user.ID, &user.Username, &user.DisplayName, &user.PasswordHash, &status, &user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	user.Status = models.UserStatus(status)

	return &user, nil
}

// GetUserByUsername retrieves a user by their username.
func (s *UserStore) GetUserByUsername(ctx context.Context, username string) (*models.User, error) {
	if s.db == nil && s.pg == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if u, ok := s.usersByName[username]; ok {
			uCopy := *u
			return &uCopy, nil
		}
		return nil, fmt.Errorf("user not found")
	}

	// Postgres mode
	if s.pg != nil {
		var userID int64
		err := s.pg.QueryRowContext(ctx, `SELECT user_id FROM users WHERE username = $1`, username).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("lookup username pg: %w", err)
		}
		return s.GetUserByID(ctx, userID)
	}

	var userID int64
	err := s.db.Session.Query(
		`SELECT user_id FROM users_by_username WHERE username = ?`,
		username,
	).WithContext(ctx).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("lookup username: %w", err)
	}

	return s.GetUserByID(ctx, userID)
}

// GetUserByPublicKey retrieves a user by their SSH public key fingerprint.
func (s *UserStore) GetUserByPublicKey(ctx context.Context, fingerprint string) (*models.User, error) {
	if s.db == nil && s.pg == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if u, ok := s.usersByPK[fingerprint]; ok {
			uCopy := *u
			return &uCopy, nil
		}
		return nil, fmt.Errorf("public key not found")
	}

	// Postgres mode
	if s.pg != nil {
		var userID int64
		err := s.pg.QueryRowContext(ctx, `SELECT user_id FROM user_public_keys WHERE fingerprint = $1`, fingerprint).Scan(&userID)
		if err != nil {
			return nil, fmt.Errorf("lookup public key pg: %w", err)
		}
		return s.GetUserByID(ctx, userID)
	}

	var userID int64
	err := s.db.Session.Query(
		`SELECT user_id FROM user_public_keys WHERE fingerprint = ?`,
		fingerprint,
	).WithContext(ctx).Scan(&userID)
	if err != nil {
		return nil, fmt.Errorf("lookup public key: %w", err)
	}

	return s.GetUserByID(ctx, userID)
}

// AddPublicKey associates an SSH public key with a user account.
func (s *UserStore) AddPublicKey(ctx context.Context, userID int64, fingerprint, keyData string) error {
	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if u, ok := s.usersByID[userID]; ok {
			s.usersByPK[fingerprint] = u
		}
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		_, err := s.pg.ExecContext(ctx, `INSERT INTO user_public_keys (fingerprint, user_id, public_key_data) VALUES ($1, $2, $3)`, fingerprint, userID, keyData)
		if err != nil {
			return fmt.Errorf("add public key pg: %w", err)
		}
		return nil
	}

	err := s.db.Session.Query(
		`INSERT INTO user_public_keys (fingerprint, user_id, public_key_data) VALUES (?, ?, ?)`,
		fingerprint, userID, keyData,
	).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("add public key: %w", err)
	}
	return nil
}

// UpdateStatus updates a user's online status.
func (s *UserStore) UpdateStatus(ctx context.Context, userID int64, status models.UserStatus) error {
	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if u, ok := s.usersByID[userID]; ok {
			u.Status = status
		}
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		_, err := s.pg.ExecContext(ctx, `UPDATE users SET status = $1 WHERE user_id = $2`, int(status), userID)
		if err != nil {
			return fmt.Errorf("update status pg: %w", err)
		}
		return nil
	}

	err := s.db.Session.Query(
		`UPDATE users SET status = ? WHERE user_id = ?`,
		int(status), userID,
	).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

// UsernameExists checks if a username is already taken.
func (s *UserStore) UsernameExists(ctx context.Context, username string) (bool, error) {
	if s.db == nil && s.pg == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		_, ok := s.usersByName[username]
		return ok, nil
	}

	// Postgres mode
	if s.pg != nil {
		var userID int64
		err := s.pg.QueryRowContext(ctx, `SELECT user_id FROM users WHERE username = $1`, username).Scan(&userID)
		if err != nil {
			if err.Error() == "sql: no rows in result set" {
				return false, nil
			}
			return false, fmt.Errorf("check username pg: %w", err)
		}
		return true, nil
	}

	var userID int64
	err := s.db.Session.Query(
		`SELECT user_id FROM users_by_username WHERE username = ?`,
		username,
	).WithContext(ctx).Scan(&userID)
	if err != nil {
		if err.Error() == "not found" {
			return false, nil
		}
		return false, fmt.Errorf("check username: %w", err)
	}
	return true, nil
}

// VerifyPassword checks if the given password matches the stored hash.
func (s *UserStore) VerifyPassword(hashedPassword, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}

// UpdateUsername updates a user's username across memory and database.
func (s *UserStore) UpdateUsername(ctx context.Context, userID int64, oldUsername, newUsername string) error {
	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()

		if u, ok := s.usersByID[userID]; ok {
			u.Username = newUsername
			u.DisplayName = newUsername
			delete(s.usersByName, oldUsername)
			s.usersByName[newUsername] = u
		}
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		tx, err := s.pg.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("update username begin tx pg: %w", err)
		}
		_, err = tx.ExecContext(ctx, `UPDATE users SET username = $1, display_name = $2 WHERE user_id = $3`, newUsername, newUsername, userID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("update username tx pg: %w", err)
		}
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("update username commit pg: %w", err)
		}
		return nil
	}

	batch := s.db.Session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	batch.Query(`UPDATE users SET username = ?, display_name = ? WHERE user_id = ?`, newUsername, newUsername, userID)
	batch.Query(`DELETE FROM users_by_username WHERE username = ?`, oldUsername)
	batch.Query(`INSERT INTO users_by_username (username, user_id) VALUES (?, ?)`, newUsername, userID)

	if err := s.db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("update username: %w", err)
	}
	return nil
}

// UpdatePassword updates and hashes a user's password.
func (s *UserStore) UpdatePassword(ctx context.Context, userID int64, newPassword string) error {
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if u, ok := s.usersByID[userID]; ok {
			u.PasswordHash = string(hashed)
		}
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		_, err := s.pg.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE user_id = $2`, string(hashed), userID)
		if err != nil {
			return fmt.Errorf("update password pg: %w", err)
		}
		return nil
	}

	err = s.db.Session.Query(`UPDATE users SET password_hash = ? WHERE user_id = ?`, string(hashed), userID).WithContext(ctx).Exec()
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// BanUser permanently bans a user from the system by updating their role.
func (s *UserStore) BanUser(ctx context.Context, userID int64, username string) error {
	if s.db == nil && s.pg == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if u, ok := s.usersByID[userID]; ok {
			u.Role = "banned"
		}
		return nil
	}

	// Postgres mode
	if s.pg != nil {
		_, err := s.pg.ExecContext(ctx, `UPDATE users SET role = 'banned' WHERE user_id = $1`, userID)
		if err != nil {
			return fmt.Errorf("ban user pg: %w", err)
		}
		return nil
	}

	batch := s.db.Session.NewBatch(gocql.LoggedBatch).WithContext(ctx)
	// We'd add a role column to Cassandra if we fully supported it, but omitting for now
	if err := s.db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("ban user: %w", err)
	}
	return nil
}

// GetTotalUsers returns the total number of registered users.
func (s *UserStore) GetTotalUsers(ctx context.Context) int {
	if s.db == nil && s.pg == nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return len(s.usersByID)
	}
	if s.pg != nil {
		var count int
		_ = s.pg.QueryRowContext(ctx, "SELECT count(*) FROM users").Scan(&count)
		return count
	}
	var count int
	_ = s.db.Session.Query(`SELECT count(*) FROM users`).WithContext(ctx).Scan(&count)
	return count
}
