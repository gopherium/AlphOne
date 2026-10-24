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

// ListContactIdentities returns the identities of the given contacts, each ordered by channel and identifier.
func (s *ContactStore) ListContactIdentities(
	ctx context.Context, contactIDs []uuid.UUID,
) ([]contact.Identity, error) {
	rows, err := s.queries.ListContactIdentities(ctx, db.ListContactIdentitiesParams{
		ContactIds: contactIDs, TenantID: sdk.TenantOrDefault(ctx),
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

// MatchIdentities returns the contact owning each identifier on channel, keyed by identifier, leaving out the unowned.
func (s *ContactStore) MatchIdentities(
	ctx context.Context, channel contact.Channel, identifiers []string,
) (map[string]contact.Contact, error) {
	rows, err := s.queries.MatchIdentities(ctx, db.MatchIdentitiesParams{
		TenantID: sdk.TenantOrDefault(ctx), Channel: string(channel), Identifiers: identifiers,
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: match identities: %w", err)
	}
	owners := make(map[string]contact.Contact, len(rows))
	for _, row := range rows {
		owners[row.Identifier] = contact.Contact{ID: row.ID, Name: row.Name, CreatedAt: row.CreatedAt}
	}
	return owners, nil
}

// IdentitiesAddedSince returns at most limit identities of channel added after the cursor and the cursor after them.
func (s *ContactStore) IdentitiesAddedSince(
	ctx context.Context, channel contact.Channel, after contact.Cursor, limit int,
) ([]contact.AddedIdentity, contact.Cursor, error) {
	rows, err := s.queries.ListIdentitiesAddedSince(ctx, db.ListIdentitiesAddedSinceParams{
		TenantID: sdk.TenantOrDefault(ctx),
		Channel:  string(channel),
		AfterXid: after.Transaction,
		AfterID:  after.Identity,
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, after, fmt.Errorf("postgres: list identities added since: %w", err)
	}
	added := make([]contact.AddedIdentity, len(rows))
	next := after
	for i, row := range rows {
		added[i] = contact.AddedIdentity{ContactID: row.ContactID, Identifier: row.Identifier, AddedAt: row.CreatedAt}
		next = contact.Cursor{Transaction: row.AddedXid, Identity: row.ID}
	}
	return added, next, nil
}

// IdentityCursorCheck returns the server's identity and whether the cursor names a transaction past its count.
func (s *ContactStore) IdentityCursorCheck(ctx context.Context, cursor contact.Cursor) (contact.CursorCheck, error) {
	check, err := s.queries.IdentityCursorCheck(ctx, cursor.Transaction)
	if err != nil {
		return contact.CursorCheck{}, fmt.Errorf("postgres: check the identity cursor: %w", err)
	}
	return contact.CursorCheck{Server: check.Server, Ahead: check.Ahead}, nil
}

// IdentityHorizon returns the cursor before every identity a transaction still open may add.
func (s *ContactStore) IdentityHorizon(ctx context.Context) (contact.Cursor, error) {
	horizon, err := s.queries.IdentityHorizon(ctx)
	if err != nil {
		return contact.Cursor{}, fmt.Errorf("postgres: identity horizon: %w", err)
	}
	return contact.Cursor{Server: horizon.Server, Transaction: horizon.Horizon}, nil
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
