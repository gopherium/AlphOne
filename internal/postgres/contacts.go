// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres/db"
)

var _ contact.Store = (*ContactStore)(nil)

// ContactStore persists contacts in the core schema.
type ContactStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewContactStore returns a [ContactStore] backed by pool.
func NewContactStore(pool *pgxpool.Pool) *ContactStore {
	return &ContactStore{pool: pool, queries: db.New(pool)}
}

// Create stores a new contact.
func (s *ContactStore) Create(ctx context.Context, c contact.Contact) error {
	err := s.queries.CreateContact(ctx, db.CreateContactParams{
		ID:        c.ID,
		Name:      c.Name,
		CreatedAt: c.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create contact: %w", err)
	}
	return nil
}

// Get returns the contact with the given id, or [contact.ErrNotFound] if
// none exists.
func (s *ContactStore) Get(ctx context.Context, id uuid.UUID) (contact.Contact, error) {
	row, err := s.queries.GetContact(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return contact.Contact{}, contact.ErrNotFound
	}
	if err != nil {
		return contact.Contact{}, fmt.Errorf("postgres: get contact: %w", err)
	}
	return contact.Contact{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}, nil
}
