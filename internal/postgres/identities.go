// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres/db"
)

// LookupIdentity returns the identity for channel and identifier, or
// [contact.ErrIdentityNotFound] if none exists.
func (s *ContactStore) LookupIdentity(
	ctx context.Context,
	channel contact.Channel,
	identifier string,
) (contact.Identity, error) {
	row, err := s.queries.GetIdentity(ctx, db.GetIdentityParams{
		Channel:    string(channel),
		Identifier: identifier,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return contact.Identity{}, contact.ErrIdentityNotFound
	}
	if err != nil {
		return contact.Identity{}, fmt.Errorf("postgres: lookup identity: %w", err)
	}
	return contact.Identity{
		ID:          row.ID,
		ContactID:   row.ContactID,
		Channel:     contact.Channel(row.Channel),
		Identifier:  row.Identifier,
		DisplayName: row.DisplayName,
		CreatedAt:   row.CreatedAt,
	}, nil
}

// CreateContactWithIdentity stores a new contact owning its
// first identity. It returns [contact.ErrIdentityExists] and leaves the
// database unchanged when the identity is already claimed.
func (s *ContactStore) CreateContactWithIdentity(
	ctx context.Context,
	c contact.Contact,
	identity contact.Identity,
) error {
	err := s.createContactWithIdentity(ctx, c, identity)
	if err != nil && !errors.Is(err, contact.ErrIdentityExists) {
		return fmt.Errorf("postgres: create contact with identity: %w", err)
	}
	return err
}

// createContactWithIdentity inserts the contact and its identity in a single transaction, returning
// [contact.ErrIdentityExists] without committing when the identity is already claimed.
func (s *ContactStore) createContactWithIdentity(
	ctx context.Context,
	c contact.Contact,
	identity contact.Identity,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.queries.WithTx(tx)
	err = qtx.CreateContact(ctx, db.CreateContactParams{
		ID:        c.ID,
		Name:      c.Name,
		CreatedAt: c.CreatedAt,
	})
	if err != nil {
		return err
	}
	rows, err := qtx.CreateIdentity(ctx, db.CreateIdentityParams{
		ID:          identity.ID,
		ContactID:   identity.ContactID,
		Channel:     string(identity.Channel),
		Identifier:  identity.Identifier,
		DisplayName: identity.DisplayName,
		CreatedAt:   identity.CreatedAt,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return contact.ErrIdentityExists
	}
	return tx.Commit(ctx)
}
