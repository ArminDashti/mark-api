package users

import (
	"context"
	"database/sql"

	"github.com/ArminDashti/mark-api/internal/models"
)

// Store loads users from PostgreSQL.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// GetByUsername returns a user by username.
func (s *Store) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	var user models.User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, email, username, password_hash, created_at, updated_at
		FROM users WHERE username = $1
	`, username).Scan(
		&user.ID, &user.Email, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
