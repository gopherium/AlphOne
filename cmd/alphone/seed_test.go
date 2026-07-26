// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
)

func testPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func countRows(t *testing.T, pool *pgxpool.Pool, table string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("counting %s: %v", table, err)
	}
	return count
}

func demoCounts(t *testing.T, pool *pgxpool.Pool) [4]int {
	t.Helper()
	return [4]int{
		countRows(t, pool, "core.contacts"),
		countRows(t, pool, "plugin_whatsapp.conversations"),
		countRows(t, pool, "plugin_whatsapp.messages"),
		countRows(t, pool, "plugin_whatsapp.media"),
	}
}

func TestSeedPopulatesTheDemoData(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	var stdout strings.Builder

	if err := seed(t.Context(), getenv, &stdout); err != nil {
		t.Fatalf("seed() error = %v, want nil", err)
	}

	if !strings.Contains(stdout.String(), "admin@example.com / password1234") {
		t.Errorf("output = %q, want it to print the demo credentials", stdout.String())
	}
	pool := testPool(t, databaseURL)
	admin, err := authkitpg.NewUserStore(pool).UserByEmail(t.Context(), "admin@example.com")
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the seeded admin", err)
	}
	if !gouncer.VerifyPassword(admin.PasswordHash, "password1234") {
		t.Error("stored password hash does not verify against the demo password")
	}
	if got, want := demoCounts(t, pool), [4]int{4, 3, 8, 1}; got != want {
		t.Errorf("demo counts = %v, want %v", got, want)
	}
	var adas int
	err = pool.QueryRow(t.Context(),
		"SELECT count(*) FROM core.contacts WHERE name = 'Ada Lovelace'").Scan(&adas)
	if err != nil || adas != 1 {
		t.Errorf("Ada Lovelace contacts = %d (err %v), want 1", adas, err)
	}
}

func TestSeedIsIdempotentAcrossRuns(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("first seed() error = %v, want nil", err)
	}
	var second strings.Builder
	if err := seed(t.Context(), getenv, &second); err != nil {
		t.Fatalf("second seed() error = %v, want nil", err)
	}

	pool := testPool(t, databaseURL)
	if got, want := demoCounts(t, pool), [4]int{4, 3, 8, 1}; got != want {
		t.Errorf("demo counts after two runs = %v, want %v", got, want)
	}
	if !strings.Contains(second.String(), "admin@example.com already exists") {
		t.Errorf("second output = %q, want it to report the existing admin", second.String())
	}
	if strings.Contains(second.String(), "password1234") {
		t.Errorf("second output = %q, want it to not repeat the demo password", second.String())
	}
}

func TestSeedValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"missing database url":   nil,
		"malformed database url": {"ALPHONE_DATABASE_URL": "not a url \x00"},
		"unreachable database":   {"ALPHONE_DATABASE_URL": unreachableDatabaseURL},
	}

	for testName, env := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			if err := seed(t.Context(), testGetenv(env), &strings.Builder{}); err == nil {
				t.Fatal("seed() error = nil, want a failure")
			}
		})
	}
}

func TestSeedReportsCoreMigrationFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(), "ALTER TABLE goose_db_version DROP COLUMN version_id"); err != nil {
		t.Fatalf("breaking the core migration table: %v", err)
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a core migration failure")
	}
}

func TestSeedReportsInvalidPluginConfiguration(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":             testDatabaseURL(t),
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "not a number",
	})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a plugin configuration failure")
	}
}

func TestSeedReportsBrokenContactStorage(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	if err := seed(t.Context(), getenv, &strings.Builder{}); err != nil {
		t.Fatalf("first seed() error = %v, want nil", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(), "DROP TABLE core.contact_identities"); err != nil {
		t.Fatalf("dropping the identities table: %v", err)
	}

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want a contact storage failure")
	}
}

func TestSeedReportsAdminStorageFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	if err := authkitpg.Migrate(t.Context(), databaseURL); err != nil {
		t.Fatalf("migrating the auth schema: %v", err)
	}
	pool := testPool(t, databaseURL)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE auth.users ADD CONSTRAINT seed_sabotage CHECK (false)"); err != nil {
		t.Fatalf("breaking the users table: %v", err)
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	if err := seed(t.Context(), getenv, &strings.Builder{}); err == nil {
		t.Fatal("seed() error = nil, want an admin storage failure")
	}
}

func TestSeedAdminValidatesThePassword(t *testing.T) {
	t.Parallel()

	_, err := seedAdmin(t.Context(), nil, "admin@example.com", "Admin", "short")

	if err == nil {
		t.Fatal("seedAdmin() error = nil, want a weak password failure")
	}
}

func TestSeedAdminReportsStorageFailure(t *testing.T) {
	t.Parallel()

	store := authkitpg.NewUserStore(testPool(t, unreachableDatabaseURL))

	_, err := seedAdmin(t.Context(), store, "admin@example.com", "Admin", "password1234")

	if err == nil {
		t.Fatal("seedAdmin() error = nil, want a storage failure")
	}
}

func TestSeedWhatsAppReportsMigrationFailure(t *testing.T) {
	t.Parallel()

	resolver := contact.NewResolver(postgres.NewContactStore(testPool(t, unreachableDatabaseURL)))

	err := seedWhatsApp(t.Context(), unreachableDatabaseURL, testGetenv(nil), resolver)

	if err == nil {
		t.Fatal("seedWhatsApp() error = nil, want a migration failure")
	}
}

func TestSeedWhatsAppReportsSeedFailure(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	resolver := contact.NewResolver(postgres.NewContactStore(testPool(t, unreachableDatabaseURL)))

	err := seedWhatsApp(t.Context(), databaseURL, testGetenv(nil), resolver)

	if err == nil {
		t.Fatal("seedWhatsApp() error = nil, want a seed failure")
	}
}
