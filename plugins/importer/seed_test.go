// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSeedStoresACommittedDemoImport(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	ada := contacts.seed("Ada Lovelace", "ada@example.com")

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	var id uuid.UUID
	var state string
	var rowCount, imported, skipped, failed int
	if err := pool.QueryRow(t.Context(),
		"SELECT id, state, row_count, imported_count, skipped_count, failed_count "+
			"FROM plugin_importer.imports",
	).Scan(&id, &state, &rowCount, &imported, &skipped, &failed); err != nil {
		t.Fatalf("reading the seeded import: %v", err)
	}
	if state != "committed" {
		t.Errorf("state = %q, want committed so the history shows a finished import", state)
	}
	if rowCount != 6 || imported != 3 || skipped != 2 || failed != 1 {
		t.Errorf("counts = %d rows, %d imported, %d skipped, %d failed, want 6, 3, 2, 1",
			rowCount, imported, skipped, failed)
	}
	if outcomes := outcomesOf(t, pool, id); slices.Compare(outcomes, []string{
		"imported", "imported", "imported", "skipped", "skipped", "failed"}) != 0 {
		t.Errorf("outcomes = %v, want three imported, two skipped, one failed", outcomes)
	}
	var columns []string
	var second string
	if err := pool.QueryRow(t.Context(),
		"SELECT columns, mapping->>'1' FROM plugin_importer.imports",
	).Scan(&columns, &second); err != nil {
		t.Fatalf("reading the seeded mapping: %v", err)
	}
	if slices.Compare(columns, []string{"Name", "Email", "Phone"}) != 0 {
		t.Errorf("columns = %v, want the header of the demo file", columns)
	}
	if second != "email" {
		t.Errorf("mapping of column 1 = %q, want email so the detail screen reads it", second)
	}
	if linked := linkOf(t, pool, 4); linked == nil || *linked != ada.ID {
		t.Errorf("skipped row links to %v, want the contact it collided with %v", linked, ada.ID)
	}
	if linked := linkOf(t, pool, 6); linked != nil {
		t.Errorf("failed row links to %v, want no contact", linked)
	}
}

func TestSeedPointsARowAtWhatAnEarlierRowStored(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	stored, collided := linkOf(t, pool, 1), linkOf(t, pool, 5)
	if collided == nil || stored == nil || *collided != *stored {
		t.Errorf("the second skipped row links to %v, want the contact row 1 stored, %v",
			collided, stored)
	}
}

func TestSeedStoresTheImportedContact(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	linked := linkOf(t, pool, 1)
	if linked == nil {
		t.Fatal("imported row links to no contact, want the one the import created")
	}
	var name string
	if err := pool.QueryRow(t.Context(),
		"SELECT name FROM core.contacts WHERE id = $1", *linked).Scan(&name); err != nil {
		t.Fatalf("reading the created contact: %v", err)
	}
	if name != "Maria Perez" {
		t.Errorf("contact name = %q, want the name the imported row carries", name)
	}
}

func TestSeedIsReadableThroughTheAPI(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	var id uuid.UUID
	if err := pool.QueryRow(t.Context(),
		"SELECT id FROM plugin_importer.imports").Scan(&id); err != nil {
		t.Fatalf("reading the seeded import: %v", err)
	}
	history := getPath(t, p, "/imports")
	if history.Code != http.StatusOK {
		t.Fatalf("history status = %d, want %d, body %s",
			history.Code, http.StatusOK, history.Body)
	}
	if !strings.Contains(history.Body.String(), "spring-leads.csv") {
		t.Errorf("history = %s, want the demo import listed", history.Body)
	}
	rows := getPath(t, p, "/imports/"+id.String()+"/rows")
	if rows.Code != http.StatusOK {
		t.Fatalf("rows status = %d, want %d, body %s", rows.Code, http.StatusOK, rows.Body)
	}
	if !strings.Contains(rows.Body.String(), "Grace Hopper") {
		t.Errorf("rows = %s, want the scripted cells served", rows.Body)
	}
}

func TestSeedLeavesAnEarlierRunAlone(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	contacts.seed("Ada Lovelace", "ada@example.com")
	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("first Seed() error = %v, want nil", err)
	}

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("second Seed() error = %v, want nil", err)
	}

	var imports, rows int
	if err := pool.QueryRow(t.Context(),
		"SELECT (SELECT count(*) FROM plugin_importer.imports), "+
			"(SELECT count(*) FROM plugin_importer.import_rows)",
	).Scan(&imports, &rows); err != nil {
		t.Fatalf("counting what the second run left: %v", err)
	}
	if imports != 1 || rows != 6 {
		t.Errorf("stored %d imports and %d rows, want 1 and 6", imports, rows)
	}
	if contacts.creates != 3 {
		t.Errorf("created %d contacts, want the second run to create none", contacts.creates)
	}
}

func TestSeedRunsWithoutTheClaimingContact(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)

	if err := p.Seed(t.Context()); err != nil {
		t.Fatalf("Seed() error = %v, want nil", err)
	}

	if linked := linkOf(t, pool, 4); linked != nil {
		t.Errorf("skipped row links to %v, want no contact when none claims the email", linked)
	}
}

func TestSeedReportsALookupFailure(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	p := newPlugin(t, cfg.URL())

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() without the importer schema error = nil, want an error")
	}
}

func TestSeedReportsADirectoryFailure(t *testing.T) {
	t.Parallel()

	p, _, contacts, _ := newCommittingPlugin(t)
	contacts.createErr = errDirectory

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want the directory failure reported")
	}
}

func TestSeedReportsAClaimLookupFailure(t *testing.T) {
	t.Parallel()

	p, _, contacts, _ := newCommittingPlugin(t)
	contacts.findErr = errDirectory

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want the claim lookup failure reported")
	}
}

func TestSeedReportsAnImportFailure(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	if _, err := pool.Exec(t.Context(),
		"ALTER TABLE plugin_importer.imports ADD CONSTRAINT no_seed "+
			"CHECK (filename <> 'spring-leads.csv')"); err != nil {
		t.Fatalf("refusing the demo filename: %v", err)
	}

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want the rejected import reported")
	}
}

func TestSeedReportsARowFailure(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	if _, err := pool.Exec(t.Context(),
		"DROP TABLE plugin_importer.import_rows"); err != nil {
		t.Fatalf("dropping the rows table: %v", err)
	}

	if err := p.Seed(t.Context()); err == nil {
		t.Fatal("Seed() error = nil, want the rejected row reported")
	}
}

// linkOf returns the contact one seeded row points at.
func linkOf(t *testing.T, pool *pgxpool.Pool, position int) *uuid.UUID {
	t.Helper()
	var linked *uuid.UUID
	if err := pool.QueryRow(t.Context(),
		"SELECT contact_id FROM plugin_importer.import_rows WHERE position = $1",
		position).Scan(&linked); err != nil {
		t.Fatalf("reading the link of row %d: %v", position, err)
	}
	return linked
}
