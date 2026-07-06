// SPDX-License-Identifier: AGPL-3.0-or-later

package postgres_test

import (
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/testdb"
)

const (
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.New(t, testdb.Config(), testdb.CoreMigrator())
}

func mustContact(t *testing.T, name string) contact.Contact {
	t.Helper()
	c, err := contact.New(name)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", name, err)
	}
	return c
}

func mustIdentity(t *testing.T, contactID uuid.UUID, channel contact.Channel, identifier string) contact.Identity {
	t.Helper()
	identity, err := contact.NewIdentity(contactID, channel, identifier, "")
	if err != nil {
		t.Fatalf("NewIdentity() error = %v, want nil", err)
	}
	return identity
}

func insertContact(t *testing.T, db *sql.DB, c contact.Contact) {
	t.Helper()
	_, err := db.Exec(
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)",
		c.ID, c.Name, c.CreatedAt,
	)
	if err != nil {
		t.Fatalf("inserting contact %q: %v", c.Name, err)
	}
}

func insertIdentity(db *sql.DB, identity contact.Identity) error {
	_, err := db.Exec(
		"INSERT INTO core.contact_identities (id, contact_id, channel, identifier, display_name, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		identity.ID, identity.ContactID, string(identity.Channel), identity.Identifier, identity.DisplayName, identity.CreatedAt,
	)
	return err
}

func TestMigrationsStoreContactWithIdentity(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	maria := mustContact(t, "María Pérez")
	insertContact(t, db, maria)

	err := insertIdentity(db, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid"))

	if err != nil {
		t.Fatalf("inserting identity: %v, want nil", err)
	}
}

func TestMigrationsEnforceIdentityUniquenessAcrossContacts(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	maria := mustContact(t, "María Pérez")
	john := mustContact(t, "John Doe")
	insertContact(t, db, maria)
	insertContact(t, db, john)
	if err := insertIdentity(db, mustIdentity(t, maria.ID, "whatsapp", "184467235@lid")); err != nil {
		t.Fatalf("inserting first identity: %v, want nil", err)
	}

	err := insertIdentity(db, mustIdentity(t, john.ID, "whatsapp", "184467235@lid"))

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		t.Fatalf("inserting duplicate identity: %v, want unique violation %s", err, uniqueViolation)
	}
}

func TestMigrationsRejectIdentityWithoutContact(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	err := insertIdentity(db, mustIdentity(t, uuid.Must(uuid.NewV7()), "whatsapp", "184467235@lid"))

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != foreignKeyViolation {
		t.Fatalf("inserting orphan identity: %v, want foreign key violation %s", err, foreignKeyViolation)
	}
}
