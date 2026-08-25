// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/sdk"
)

// foreignKeyViolation is the PostgreSQL error code for a broken reference.
const foreignKeyViolation = "23503"

// isMissingContact reports whether err is the contact foreign key violation.
func isMissingContact(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolation
}

// ListContactIdentities returns the contact's identities ordered by channel
// and identifier.
func (s *ContactStore) ListContactIdentities(
	ctx context.Context, contactID uuid.UUID,
) ([]contact.Identity, error) {
	rows, err := s.queries.ListContactIdentities(ctx, db.ListContactIdentitiesParams{
		ContactID: contactID, TenantID: sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list contact identities: %w", err)
	}
	identities := make([]contact.Identity, len(rows))
	for i, row := range rows {
		identities[i] = contact.Identity{
			ID:          row.ID,
			ContactID:   row.ContactID,
			Channel:     contact.Channel(row.Channel),
			Identifier:  row.Identifier,
			DisplayName: row.DisplayName,
			CreatedAt:   row.CreatedAt,
		}
	}
	return identities, nil
}

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
		TenantID:   sdk.TenantOrDefault(ctx),
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

// AddIdentity attaches an identity to its contact, reporting the current
// owner through a [contact.IdentityExistsError] when the pair is claimed.
func (s *ContactStore) AddIdentity(ctx context.Context, identity contact.Identity) error {
	row, err := s.queries.AddIdentity(ctx, db.AddIdentityParams{
		ID:          identity.ID,
		ContactID:   identity.ContactID,
		Channel:     string(identity.Channel),
		Identifier:  identity.Identifier,
		DisplayName: identity.DisplayName,
		CreatedAt:   identity.CreatedAt,
		TenantID:    sdk.TenantOrDefault(ctx),
	})
	if isMissingContact(err) {
		return contact.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("postgres: add identity: %w", err)
	}
	if !row.Created {
		return contact.IdentityExistsError{OwnerID: row.OwnerID}
	}
	return nil
}

// DeleteIdentity removes the contact's identity with the given id, or
// reports [contact.ErrIdentityNotFound].
func (s *ContactStore) DeleteIdentity(ctx context.Context, contactID, identityID uuid.UUID) error {
	rows, err := s.queries.DeleteIdentity(ctx, db.DeleteIdentityParams{
		ID:        identityID,
		ContactID: contactID,
		TenantID:  sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return fmt.Errorf("postgres: delete identity: %w", err)
	}
	if rows == 0 {
		return contact.ErrIdentityNotFound
	}
	return nil
}

// CreateContactWithIdentity stores a new contact owning its
// first identity. It returns [contact.ErrIdentityExists] and leaves the
// database unchanged when the identity is already claimed.
func (s *ContactStore) CreateContactWithIdentity(
	ctx context.Context,
	c contact.Contact,
	identity contact.Identity,
) error {
	return s.CreateContactWithIdentities(ctx, c, []contact.Identity{identity})
}

// CreateContactWithIdentities stores a new contact owning every given
// identity. It reports the owner through a [contact.IdentityExistsError]
// and leaves the database unchanged when any pair is already claimed.
func (s *ContactStore) CreateContactWithIdentities(
	ctx context.Context,
	c contact.Contact,
	identities []contact.Identity,
) error {
	err := s.createContactWithIdentities(ctx, c, identities)
	if err != nil && !errors.Is(err, contact.ErrIdentityExists) {
		return fmt.Errorf("postgres: create contact with identities: %w", err)
	}
	return err
}

// createContactWithIdentities inserts the contact and its identities in one
// transaction, committing nothing when any identity is already claimed.
func (s *ContactStore) createContactWithIdentities(
	ctx context.Context,
	c contact.Contact,
	identities []contact.Identity,
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
		TenantID:  sdk.TenantOrDefault(ctx),
	})
	if err != nil {
		return err
	}
	if err := attachIdentities(ctx, qtx, identities); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// attachIdentities inserts every identity through qtx, reporting the owner
// of any already claimed pair.
func attachIdentities(ctx context.Context, qtx *db.Queries, identities []contact.Identity) error {
	for _, identity := range identities {
		row, err := qtx.AddIdentity(ctx, db.AddIdentityParams{
			ID:          identity.ID,
			ContactID:   identity.ContactID,
			Channel:     string(identity.Channel),
			Identifier:  identity.Identifier,
			DisplayName: identity.DisplayName,
			CreatedAt:   identity.CreatedAt,
			TenantID:    sdk.TenantOrDefault(ctx),
		})
		if err != nil {
			return err
		}
		if !row.Created {
			return contact.IdentityExistsError{OwnerID: row.OwnerID}
		}
	}
	return nil
}
