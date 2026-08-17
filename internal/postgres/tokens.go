// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres/db"
)

// TokenStore persists API tokens in the core schema.
type TokenStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewTokenStore returns a [TokenStore] backed by pool.
func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool, queries: db.New(pool)}
}

// Create stores a new token.
func (s *TokenStore) Create(ctx context.Context, t apitoken.Token) error {
	err := s.queries.CreateAPIToken(ctx, db.CreateAPITokenParams{
		ID:         t.ID,
		UserID:     t.UserID,
		Name:       t.Name,
		TokenHash:  t.Hash,
		Scopes:     t.Scopes.String(),
		CreatedAt:  t.CreatedAt,
		LastUsedAt: optionalTime(t.LastUsedAt),
		ExpiresAt:  optionalTime(t.ExpiresAt),
	})
	if err != nil {
		return fmt.Errorf("postgres: create api token: %w", err)
	}
	return nil
}

// ByHash returns the token stored under hash, or [apitoken.ErrNotFound] if
// none exists.
func (s *TokenStore) ByHash(ctx context.Context, hash string) (apitoken.Token, error) {
	row, err := s.queries.GetAPITokenByHash(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return apitoken.Token{}, apitoken.ErrNotFound
	}
	if err != nil {
		return apitoken.Token{}, fmt.Errorf("postgres: get api token: %w", err)
	}
	return tokenFromRow(row), nil
}

// TouchLastUsed records at as the moment the token was last presented.
func (s *TokenStore) TouchLastUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	err := s.queries.TouchAPIToken(ctx, db.TouchAPITokenParams{
		ID:         id,
		LastUsedAt: optionalTime(at),
	})
	if err != nil {
		return fmt.Errorf("postgres: touch api token: %w", err)
	}
	return nil
}

// ListForUser returns the tokens of one user, newest first.
func (s *TokenStore) ListForUser(ctx context.Context, userID uuid.UUID) ([]apitoken.Token, error) {
	rows, err := s.queries.ListAPITokensForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list api tokens: %w", err)
	}
	tokens := make([]apitoken.Token, 0, len(rows))
	for _, row := range rows {
		tokens = append(tokens, tokenFromRow(row))
	}
	return tokens, nil
}

// Revoke deletes one token of userID, or reports [apitoken.ErrNotFound] when
// the user owns no such token.
func (s *TokenStore) Revoke(ctx context.Context, userID, id uuid.UUID) error {
	deleted, err := s.queries.RevokeAPIToken(ctx, db.RevokeAPITokenParams{ID: id, UserID: userID})
	if err != nil {
		return fmt.Errorf("postgres: revoke api token: %w", err)
	}
	if deleted == 0 {
		return apitoken.ErrNotFound
	}
	return nil
}

// tokenFromRow maps a stored row onto the domain token.
func tokenFromRow(row db.CoreApiToken) apitoken.Token {
	return apitoken.Token{
		ID:         row.ID,
		UserID:     row.UserID,
		Name:       row.Name,
		Hash:       row.TokenHash,
		Scopes:     apitoken.ParseScopes(row.Scopes),
		CreatedAt:  row.CreatedAt,
		LastUsedAt: row.LastUsedAt.Time,
		ExpiresAt:  row.ExpiresAt.Time,
	}
}

// optionalTime maps the zero time onto NULL.
func optionalTime(at time.Time) pgtype.Timestamptz {
	if at.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: at, Valid: true}
}
