// SPDX-License-Identifier: Elastic-2.0

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

// ListContacts returns contacts after the given cursor position in name
// order, optionally narrowed by a search query and its digits.
func (s *ContactStore) ListContacts(
	ctx context.Context, query, digits, afterName string, afterID uuid.UUID, limit int,
) ([]contact.Contact, error) {
	rows, err := s.queries.ListContacts(ctx, db.ListContactsParams{
		AfterName: afterName,
		AfterID:   afterID,
		Query:     query,
		Digits:    digits,
		RowLimit:  int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list contacts: %w", err)
	}
	contacts := make([]contact.Contact, len(rows))
	for i, row := range rows {
		contacts[i] = contact.Contact{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}
	}
	return contacts, nil
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
