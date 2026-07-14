// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres/db"
)

var _ gouncer.Store = (*UserStore)(nil)

const uniqueViolationCode = "23505"

// UserStore persists users and their sessions in the core schema.
type UserStore struct {
	queries *db.Queries
}

// NewUserStore returns a [UserStore] backed by pool.
func NewUserStore(pool *pgxpool.Pool) *UserStore {
	return &UserStore{queries: db.New(pool)}
}

// CreateUser stores a new user, or [gouncer.ErrEmailTaken] when the email is
// already in use.
func (s *UserStore) CreateUser(ctx context.Context, u gouncer.User) error {
	err := s.queries.CreateUser(ctx, db.CreateUserParams{
		ID:           u.ID,
		Email:        u.Email,
		Name:         u.Name,
		PasswordHash: u.PasswordHash,
		Disabled:     u.Disabled,
		CreatedAt:    u.CreatedAt,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
		return gouncer.ErrEmailTaken
	}
	if err != nil {
		return fmt.Errorf("postgres: create user: %w", err)
	}
	return nil
}

// UserByEmail returns the user owning the email, or [gouncer.ErrUserNotFound]
// if none exists.
func (s *UserStore) UserByEmail(ctx context.Context, email string) (gouncer.User, error) {
	row, err := s.queries.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		return gouncer.User{}, gouncer.ErrUserNotFound
	}
	if err != nil {
		return gouncer.User{}, fmt.Errorf("postgres: get user by email: %w", err)
	}
	return userFromRow(row), nil
}

// CreateSession stores a login session.
func (s *UserStore) CreateSession(ctx context.Context, session gouncer.Session) error {
	err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash: session.TokenHash,
		UserID:    session.UserID,
		CreatedAt: session.CreatedAt,
		ExpiresAt: session.ExpiresAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create session: %w", err)
	}
	return nil
}

// UserBySession returns the enabled user owning an unexpired session with
// the given token hash, or [gouncer.ErrSessionNotFound].
func (s *UserStore) UserBySession(ctx context.Context, tokenHash []byte, now time.Time) (gouncer.User, error) {
	row, err := s.queries.GetUserBySession(ctx, db.GetUserBySessionParams{
		TokenHash: tokenHash,
		ExpiresAt: now,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gouncer.User{}, gouncer.ErrSessionNotFound
	}
	if err != nil {
		return gouncer.User{}, fmt.Errorf("postgres: get user by session: %w", err)
	}
	return userFromRow(row), nil
}

// ListUsers returns every user ordered by name.
func (s *UserStore) ListUsers(ctx context.Context) ([]gouncer.User, error) {
	rows, err := s.queries.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	users := make([]gouncer.User, len(rows))
	for i, row := range rows {
		users[i] = userFromRow(row)
	}
	return users, nil
}

// SetUserDisabled updates whether the user may log in, or returns
// [gouncer.ErrUserNotFound] when no such user exists.
func (s *UserStore) SetUserDisabled(ctx context.Context, id uuid.UUID, disabled bool) error {
	count, err := s.queries.SetUserDisabled(ctx, db.SetUserDisabledParams{ID: id, Disabled: disabled})
	if err != nil {
		return fmt.Errorf("postgres: set user disabled: %w", err)
	}
	if count == 0 {
		return gouncer.ErrUserNotFound
	}
	return nil
}

// DeleteSession removes the session with the given token hash, if any.
func (s *UserStore) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if err := s.queries.DeleteSession(ctx, tokenHash); err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	return nil
}

// userFromRow converts a generated user row into the domain user.
func userFromRow(row db.CoreUser) gouncer.User {
	return gouncer.User{
		ID:           row.ID,
		Email:        row.Email,
		Name:         row.Name,
		PasswordHash: row.PasswordHash,
		Disabled:     row.Disabled,
		CreatedAt:    row.CreatedAt,
	}
}
