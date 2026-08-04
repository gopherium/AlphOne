// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/peterldowns/pgtestdb"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/testdb"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

var (
	_ sdk.Plugin        = (*importer.Plugin)(nil)
	_ sdk.Migrator      = (*importer.Plugin)(nil)
	_ sdk.RouteProvider = (*importer.Plugin)(nil)
)

const uniqueViolation = "23505"

const checkViolation = "23514"

const unreachableDatabaseURL = "postgres://postgres:alphone@localhost:9/postgres?sslmode=disable&connect_timeout=1"

func newTestDatabase(t *testing.T) *pgtestdb.Config {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	return pgtestdb.Custom(t, testdb.Config(), testdb.CoreMigrator())
}

func newAssertionPool(t *testing.T, url string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), url)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func newPlugin(t *testing.T, databaseURL string) *importer.Plugin {
	t.Helper()
	p, err := importer.Register(sdk.Deps{DatabaseURL: databaseURL})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	return p
}

// newMigratedDatabase returns an assertion pool over a database holding the
// core and importer schemas.
func newMigratedDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return newAssertionPool(t, cfg.URL())
}

// seedImport stores one import with one pending row linked to a contact and
// returns the import, row, and contact ids.
func seedImport(t *testing.T, pool *pgxpool.Pool) (uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()
	maria, err := contact.New("Maria Perez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, $3)",
		maria.ID, maria.Name, maria.CreatedAt,
	); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	importID := uuid.Must(uuid.NewV7())
	now := time.Now().UTC()
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO plugin_importer.imports (id, user_id, filename, columns, mapping, "+
			"state, row_count, imported_count, skipped_count, failed_count, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
		importID, uuid.Must(uuid.NewV7()), "contacts.csv", []string{"Name", "Email"},
		`{"0":"name","1":"email"}`, "ready", 1, 0, 0, 0, now,
	); err != nil {
		t.Fatalf("inserting import: %v", err)
	}
	rowID := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, "+
			"outcome, reason, contact_id) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		rowID, importID, 1, `["Maria Perez","maria@example.com"]`, "pending", nil, maria.ID,
	); err != nil {
		t.Fatalf("inserting import row: %v", err)
	}
	return importID, rowID, maria.ID
}

func TestRegisterRejectsMalformedDatabaseURL(t *testing.T) {
	t.Parallel()

	p, err := importer.Register(sdk.Deps{DatabaseURL: "://not-a-url"})

	if err == nil {
		t.Fatal("Register() error = nil, want a parse error")
	}
	if p != nil {
		t.Errorf("Register() = %v, want nil on failure", p)
	}
}

func TestPluginIdentityAndLifecycle(t *testing.T) {
	t.Parallel()

	p, err := importer.Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if got := p.ID(); got != "importer" {
		t.Errorf("ID() = %q, want %q", got, "importer")
	}
	if err := p.Start(t.Context()); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestStopWithoutStartReleasesThePlugin(t *testing.T) {
	t.Parallel()

	p, err := importer.Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() without Start error = %v, want nil", err)
	}
}

func TestMigrateCreatesTheImportTables(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	p := newPlugin(t, cfg.URL())

	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("second Migrate() error = %v, want idempotent nil", err)
	}

	importID, rowID, _ := seedImport(t, pool)

	var secondColumn, mappedField, firstCell string
	if err := pool.QueryRow(t.Context(),
		"SELECT i.columns[2], i.mapping->>'0', r.cells->>0 "+
			"FROM plugin_importer.imports i "+
			"JOIN plugin_importer.import_rows r ON r.import_id = i.id "+
			"WHERE i.id = $1",
		importID,
	).Scan(&secondColumn, &mappedField, &firstCell); err != nil {
		t.Fatalf("reading structured columns: %v", err)
	}
	if secondColumn != "Email" || mappedField != "name" || firstCell != "Maria Perez" {
		t.Errorf("structured reads = %q, %q, %q, want Email, name, Maria Perez",
			secondColumn, mappedField, firstCell)
	}

	_, err := pool.Exec(t.Context(),
		"INSERT INTO plugin_importer.import_rows (id, import_id, position, cells, outcome) "+
			"VALUES ($1, $2, $3, $4, $5)",
		uuid.Must(uuid.NewV7()), importID, 1, `[]`, "pending",
	)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != uniqueViolation {
		t.Fatalf("inserting duplicate position: %v, want unique violation %s", err, uniqueViolation)
	}

	if _, err := pool.Exec(t.Context(),
		"DELETE FROM plugin_importer.imports WHERE id = $1", importID,
	); err != nil {
		t.Fatalf("deleting import: %v", err)
	}
	var rowSurvives bool
	if err := pool.QueryRow(t.Context(),
		"SELECT EXISTS (SELECT 1 FROM plugin_importer.import_rows WHERE id = $1)", rowID,
	).Scan(&rowSurvives); err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	if rowSurvives {
		t.Error("import row survived its import, want the delete to cascade")
	}
}

func TestContactDeletionUnlinksImportRows(t *testing.T) {
	t.Parallel()

	pool := newMigratedDatabase(t)
	_, rowID, contactID := seedImport(t, pool)

	if _, err := pool.Exec(t.Context(),
		"DELETE FROM core.contacts WHERE id = $1", contactID,
	); err != nil {
		t.Fatalf("deleting contact: %v", err)
	}

	var linkIsNull bool
	if err := pool.QueryRow(t.Context(),
		"SELECT contact_id IS NULL FROM plugin_importer.import_rows WHERE id = $1", rowID,
	).Scan(&linkIsNull); err != nil {
		t.Fatalf("reading import row: %v", err)
	}
	if !linkIsNull {
		t.Error("contact_id kept its value, want it nulled when the contact goes")
	}
}

func TestImportStateAndOutcomeAreConstrained(t *testing.T) {
	t.Parallel()

	pool := newMigratedDatabase(t)
	importID, _, _ := seedImport(t, pool)

	var pgErr *pgconn.PgError
	_, err := pool.Exec(t.Context(),
		"UPDATE plugin_importer.imports SET state = 'exploded' WHERE id = $1", importID)
	if !errors.As(err, &pgErr) || pgErr.Code != checkViolation {
		t.Fatalf("setting an unknown state: %v, want check violation %s", err, checkViolation)
	}

	_, err = pool.Exec(t.Context(),
		"UPDATE plugin_importer.import_rows SET outcome = 'exploded' WHERE import_id = $1",
		importID)
	if !errors.As(err, &pgErr) || pgErr.Code != checkViolation {
		t.Fatalf("setting an unknown outcome: %v, want check violation %s", err, checkViolation)
	}
}

func TestImporterReferencesOnlyItselfAndCore(t *testing.T) {
	t.Parallel()

	pool := newMigratedDatabase(t)

	rows, err := pool.Query(t.Context(),
		"SELECT DISTINCT ref_ns.nspname "+
			"FROM pg_constraint c "+
			"JOIN pg_class src ON src.oid = c.conrelid "+
			"JOIN pg_namespace src_ns ON src_ns.oid = src.relnamespace "+
			"JOIN pg_class ref ON ref.oid = c.confrelid "+
			"JOIN pg_namespace ref_ns ON ref_ns.oid = ref.relnamespace "+
			"WHERE src_ns.nspname = 'plugin_importer' AND c.contype = 'f'")
	if err != nil {
		t.Fatalf("listing referenced schemas: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var schema string
		if err := rows.Scan(&schema); err != nil {
			t.Fatalf("scanning schema: %v", err)
		}
		if schema != "core" && schema != "plugin_importer" {
			t.Errorf("import tables reference schema %q, want only core and plugin_importer", schema)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating schemas: %v", err)
	}
}

func TestMigrateOwnsItsGooseVersionTable(t *testing.T) {
	t.Parallel()

	pool := newMigratedDatabase(t)

	var latestVersion int64
	if err := pool.QueryRow(t.Context(),
		"SELECT max(version_id) FROM plugin_importer.goose_db_version",
	).Scan(&latestVersion); err != nil {
		t.Fatalf("reading the version table: %v", err)
	}
	if latestVersion != 1 {
		t.Errorf("latest version = %d, want 1 for the single migration", latestVersion)
	}
}

func TestMigrateReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	p := newPlugin(t, unreachableDatabaseURL)

	if err := p.Migrate(t.Context()); err == nil {
		t.Fatal("Migrate() on unreachable database error = nil, want an error")
	}
}
