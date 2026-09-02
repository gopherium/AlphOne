// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
)

// placedAdminTenant stands the seeded admin in a fresh tenant, answering the tenant id and a pool on the database.
func placedAdminTenant(t *testing.T, databaseURL string) (uuid.UUID, *pgxpool.Pool) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	admin, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("reading the seeded admin: %v", err)
	}
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", admin.ID, acme); err != nil {
		t.Fatalf("placing the admin: %v", err)
	}
	return acme, pool
}

func TestRunPlacesAnInvitedAccountInTheInvitersTenant(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	addr := freeAddr(t)
	databaseURL := testDatabaseURL(t)
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL": databaseURL,
			"ALPHONE_ADDR":         addr,
		}), io.Discard, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })

	baseURL := "http://" + addr
	waitForServer(t, baseURL)
	seedAdmin(t, databaseURL)
	acme, pool := placedAdminTenant(t, databaseURL)
	session := loginSession(t, baseURL)

	invited := postGraphAuthed(t, ctx, session, baseURL,
		`{"query":"mutation { invite(email: \"maria@example.com\", name: \"Maria Perez\") { delivered } }"}`)
	if !strings.Contains(invited, `"delivered":false`) {
		t.Fatalf("invite = %q, want it answered for hand delivery without a relay", invited)
	}

	invitee, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "maria@example.com")
	if err != nil {
		t.Fatalf("reading the invited account: %v", err)
	}
	placed, err := postgres.NewTenantStore(pool).TenantForUser(t.Context(), invitee.ID)
	if err != nil {
		t.Fatalf("TenantForUser() error = %v, want nil", err)
	}
	if placed.ID != acme {
		t.Errorf("the invited account stands in %s, want the inviter's tenant %s", placed.ID, acme)
	}
}
