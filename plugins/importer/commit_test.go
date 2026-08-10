// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

var errDirectory = errors.New("the contact directory is unreachable")

// directory answers contact lookups from an in-memory set of identities,
// storing every contact it creates so the import row link stays valid.
type directory struct {
	mu         sync.Mutex
	pool       *pgxpool.Pool
	contacts   map[uuid.UUID]sdk.Contact
	identities map[sdk.Identity]uuid.UUID
	findErr    error
	createErr  error
	refused    string
	hideClaims bool
	creates    int
}

func newDirectory() *directory {
	return &directory{
		contacts:   map[uuid.UUID]sdk.Contact{},
		identities: map[sdk.Identity]uuid.UUID{},
	}
}

// store writes a contact to core so an import row may reference it.
func (d *directory) store(owner sdk.Contact) {
	if d.pool == nil {
		return
	}
	if _, err := d.pool.Exec(context.Background(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, $2, now())",
		owner.ID, owner.Name); err != nil {
		panic(err)
	}
}

// key returns the lookup key an identity is stored under.
func key(channel sdk.Channel, identifier string) sdk.Identity {
	return sdk.Identity{Channel: channel, Identifier: identifier}
}

// seed stores a contact owning one email identity.
func (d *directory) seed(name, email string) sdk.Contact {
	owner := sdk.Contact{ID: uuid.Must(uuid.NewV7()), Name: name}
	d.contacts[owner.ID] = owner
	d.identities[key("email", email)] = owner.ID
	d.store(owner)
	return owner
}

func (d *directory) FindByIdentity(
	_ context.Context, channel sdk.Channel, identifier string,
) (sdk.Contact, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.findErr != nil {
		return sdk.Contact{}, false, d.findErr
	}
	if d.refused != "" && identifier == d.refused {
		return sdk.Contact{}, false, fmt.Errorf("%w: unusable identifier", sdk.ErrInvalidContact)
	}
	ownerID, ok := d.identities[key(channel, identifier)]
	if !ok || d.hideClaims {
		return sdk.Contact{}, false, nil
	}
	return d.contacts[ownerID], true, nil
}

func (d *directory) CreateWithIdentities(
	_ context.Context, name string, identities []sdk.Identity,
) (sdk.Contact, bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.creates++
	if d.createErr != nil {
		return sdk.Contact{}, false, d.createErr
	}
	for _, identity := range identities {
		if ownerID, ok := d.identities[key(identity.Channel, identity.Identifier)]; ok {
			return d.contacts[ownerID], false, nil
		}
	}
	created := sdk.Contact{ID: uuid.Must(uuid.NewV7()), Name: name}
	d.contacts[created.ID] = created
	for _, identity := range identities {
		d.identities[key(identity.Channel, identity.Identifier)] = created.ID
	}
	d.store(created)
	return created, true, nil
}

// recorder collects the events a plugin publishes.
type recorder struct {
	mu    sync.Mutex
	names []string
	data  []map[string]any
}

func (r *recorder) Publish(_ context.Context, name string, data map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.names = append(r.names, name)
	r.data = append(r.data, data)
}

// newCommittingPlugin returns a migrated plugin wired to a directory and an event recorder.
func newCommittingPlugin(t *testing.T) (
	*importer.Plugin, *pgxpool.Pool, *directory, *recorder,
) {
	t.Helper()
	cfg := newTestDatabase(t)
	contacts := newDirectory()
	events := &recorder{}
	p, err := importer.Register(sdk.Deps{
		DatabaseURL: cfg.URL(),
		Contacts:    contacts,
		Events:      events,
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	pool := newAssertionPool(t, cfg.URL())
	contacts.pool = pool
	return p, pool, contacts, events
}

// outcomesOf returns each row outcome of an import in position order.
func outcomesOf(t *testing.T, pool *pgxpool.Pool, importID uuid.UUID) []string {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		"SELECT outcome FROM plugin_importer.import_rows WHERE import_id = $1 ORDER BY position",
		importID)
	if err != nil {
		t.Fatalf("reading outcomes: %v", err)
	}
	defer rows.Close()
	var outcomes []string
	for rows.Next() {
		var outcome string
		if err := rows.Scan(&outcome); err != nil {
			t.Fatalf("scanning outcome: %v", err)
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes
}

// mixedCSV holds a row that dedups, a ragged row, a row without an identity, and two that import.
const mixedCSV = "Name,Email\n" +
	"Maria Perez,maria@example.com\n" +
	"Ana Lopez\n" +
	"Juan Ruiz,juan@example.com\n" +
	"Rosa Diaz,\n" +
	"Luis Sanz,luis@example.com\n"

func TestCommitImportsSkipsAndFailsEachRow(t *testing.T) {
	t.Parallel()

	p, pool, contacts, events := newCommittingPlugin(t)
	claimed := contacts.seed("Maria Perez", "maria@example.com")
	id := uploadNamed(t, p, "mixed.csv", mixedCSV)
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	want := []string{"skipped", "failed", "imported", "failed", "imported"}
	if diff := slices.Compare(want, outcomesOf(t, pool, id)); diff != 0 {
		t.Errorf("outcomes = %v, want %v", outcomesOf(t, pool, id), want)
	}
	var state string
	var imported, skipped, failed int
	if err := pool.QueryRow(t.Context(),
		"SELECT state, imported_count, skipped_count, failed_count "+
			"FROM plugin_importer.imports WHERE id = $1", id,
	).Scan(&state, &imported, &skipped, &failed); err != nil {
		t.Fatalf("reading the import: %v", err)
	}
	if state != "committed" {
		t.Errorf("state = %q, want committed", state)
	}
	if imported != 2 || skipped != 1 || failed != 2 {
		t.Errorf("counts = %d imported, %d skipped, %d failed, want 2, 1, 2", imported, skipped, failed)
	}
	var owner *uuid.UUID
	if err := pool.QueryRow(t.Context(),
		"SELECT contact_id FROM plugin_importer.import_rows WHERE import_id = $1 AND position = 1", id,
	).Scan(&owner); err != nil {
		t.Fatalf("reading the skipped row: %v", err)
	}
	if owner == nil || *owner != claimed.ID {
		t.Errorf("skipped row owner = %v, want the claiming contact %v", owner, claimed.ID)
	}
	if got := slices.Contains(events.names, "import.completed"); !got {
		t.Errorf("published %v, want import.completed", events.names)
	}
}

func TestCommitAnnouncesTheCompletedImport(t *testing.T) {
	t.Parallel()

	p, _, _, events := newCommittingPlugin(t)
	id := uploadNamed(t, p, "two.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	if len(events.names) != 1 || events.names[0] != "import.completed" {
		t.Fatalf("published %v, want exactly one import.completed", events.names)
	}
	data := events.data[0]
	if data["id"] != id.String() {
		t.Errorf("data id = %v, want %v", data["id"], id)
	}
	if data["imported"] != 1 || data["skipped"] != 0 {
		t.Errorf("data = %v, want one imported and none skipped", data)
	}
}

func TestCommitStoresTheIdentitiesNormalized(t *testing.T) {
	t.Parallel()

	p, _, contacts, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "case.csv", "Name,Email\nMaria Perez,MARIA@Example.COM\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	if _, found, _ := contacts.FindByIdentity(t.Context(), "email", "MARIA@Example.COM"); !found {
		t.Error("the identity was not handed to the directory as written in the file")
	}
}

func TestCommitSkipsARowSharingAnIdentityWithAnEarlierRow(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "twins.csv",
		"Name,Email\nMaria Perez,shared@example.com\nMaria P,shared@example.com\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	want := []string{"imported", "skipped"}
	if diff := slices.Compare(want, outcomesOf(t, pool, id)); diff != 0 {
		t.Errorf("outcomes = %v, want %v", outcomesOf(t, pool, id), want)
	}
}

func TestCommitNeverOffersAKnownContactForCreation(t *testing.T) {
	t.Parallel()

	p, _, contacts, _ := newCommittingPlugin(t)
	contacts.seed("Maria Perez", "maria@example.com")
	id := uploadNamed(t, p, "known.csv", "Name,Email\nMaria P,maria@example.com\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	if contacts.creates != 0 {
		t.Errorf("offered %d contacts for creation, want the dedup lookup to spare the write",
			contacts.creates)
	}
}

func TestCommitSkipsARowClaimedWhileTheImportRuns(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	claimed := contacts.seed("Maria Perez", "maria@example.com")
	contacts.hideClaims = true
	id := uploadNamed(t, p, "race.csv", "Name,Email\nMaria P,maria@example.com\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)

	var outcome string
	var owner *uuid.UUID
	if err := pool.QueryRow(t.Context(),
		"SELECT outcome, contact_id FROM plugin_importer.import_rows WHERE import_id = $1", id,
	).Scan(&outcome, &owner); err != nil {
		t.Fatalf("reading the row: %v", err)
	}
	if outcome != "skipped" {
		t.Errorf("outcome = %q, want skipped when the create lost the race", outcome)
	}
	if owner == nil || *owner != claimed.ID {
		t.Errorf("owner = %v, want the contact that claimed it %v", owner, claimed.ID)
	}
}

func TestCommitFinishesOnlyThePendingRowsOnAResume(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "resume.csv",
		"Name,Email\nMaria Perez,maria@example.com\nAna Lopez,ana@example.com\n")
	mapNameAndEmail(t, p, id)
	settled := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.contacts (id, name, created_at) VALUES ($1, 'Maria Perez', now())", settled,
	); err != nil {
		t.Fatalf("seeding a contact: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"UPDATE plugin_importer.imports SET state = 'committing' WHERE id = $1", id,
	); err != nil {
		t.Fatalf("moving the import into committing: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"UPDATE plugin_importer.import_rows SET outcome = 'imported', contact_id = $2 "+
			"WHERE import_id = $1 AND position = 1", id, settled,
	); err != nil {
		t.Fatalf("settling the first row: %v", err)
	}

	mustCommit(t, p, id)

	var owner uuid.UUID
	if err := pool.QueryRow(t.Context(),
		"SELECT contact_id FROM plugin_importer.import_rows WHERE import_id = $1 AND position = 1", id,
	).Scan(&owner); err != nil {
		t.Fatalf("reading the settled row: %v", err)
	}
	if owner != settled {
		t.Errorf("the settled row was recommitted, contact_id = %v, want %v", owner, settled)
	}
	want := []string{"imported", "imported"}
	if diff := slices.Compare(want, outcomesOf(t, pool, id)); diff != 0 {
		t.Errorf("outcomes = %v, want %v", outcomesOf(t, pool, id), want)
	}
}

func TestCommitRefusesAnImportThatIsAlreadyCommitted(t *testing.T) {
	t.Parallel()

	p, _, _, events := newCommittingPlugin(t)
	id := uploadNamed(t, p, "once.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)
	mustCommit(t, p, id)

	_, err := commitImport(t, p, id)

	if code := refusalCode(t, err); code != "CONFLICT" {
		t.Fatalf("replay code = %q, want CONFLICT", code)
	}
	if len(events.names) != 1 {
		t.Errorf("published %v, want the replay to announce nothing", events.names)
	}
}

func TestCommitRefusesAnImportWithoutAMapping(t *testing.T) {
	t.Parallel()

	p, _, _, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "unmapped.csv", "Name,Email\nMaria Perez,maria@example.com\n")

	_, err := commitImport(t, p, id)

	if code := refusalCode(t, err); code != "VALIDATION" {
		t.Fatalf("code = %q, want VALIDATION", code)
	}
}

// refusedCSV holds a row that imports and a row whose contact detail is unusable.
const refusedCSV = "Name,Email\n" +
	"Maria Perez,maria@example.com\n" +
	"Ana Lopez,n/a\n"

func TestCommitFailsOnlyTheRowTheHostRefuses(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "refused.csv", refusedCSV)
	mapNameAndEmail(t, p, id)
	contacts.refused = "n/a"

	mustCommit(t, p, id)

	want := []string{"imported", "failed"}
	if diff := slices.Compare(want, outcomesOf(t, pool, id)); diff != 0 {
		t.Errorf("outcomes = %v, want %v", outcomesOf(t, pool, id), want)
	}
	var state string
	var imported, failed int
	if err := pool.QueryRow(t.Context(),
		"SELECT state, imported_count, failed_count FROM plugin_importer.imports WHERE id = $1", id,
	).Scan(&state, &imported, &failed); err != nil {
		t.Fatalf("reading the import: %v", err)
	}
	if state != "committed" {
		t.Errorf("state = %q, want committed so one bad cell does not strand the import", state)
	}
	if imported != 1 || failed != 1 {
		t.Errorf("counts = %d imported, %d failed, want 1, 1", imported, failed)
	}
	var reason *string
	if err := pool.QueryRow(t.Context(),
		"SELECT reason FROM plugin_importer.import_rows WHERE import_id = $1 AND position = 2", id,
	).Scan(&reason); err != nil {
		t.Fatalf("reading the refused row: %v", err)
	}
	if reason == nil || *reason == "" {
		t.Errorf("refused row reason = %v, want a reason naming the unusable detail", reason)
	}
}

func TestCommitReportsADirectoryFailure(t *testing.T) {
	t.Parallel()

	p, pool, contacts, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "broken.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)
	contacts.findErr = errDirectory

	_, err := commitImport(t, p, id)

	if err == nil {
		t.Fatal("ImportCommit() error = nil, want the directory failure")
	}
	var state string
	if err := pool.QueryRow(t.Context(),
		"SELECT state FROM plugin_importer.imports WHERE id = $1", id).Scan(&state); err != nil {
		t.Fatalf("reading the import: %v", err)
	}
	if state != "committing" {
		t.Errorf("state = %q, want committing so the import stays resumable", state)
	}
}

func TestCommitReportsACreateFailure(t *testing.T) {
	t.Parallel()

	p, _, contacts, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "broken.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)
	contacts.createErr = errDirectory

	_, err := commitImport(t, p, id)

	if err == nil {
		t.Fatal("ImportCommit() error = nil, want the create failure")
	}
}

func TestCommitReportsAFailureAtEachWrite(t *testing.T) {
	t.Parallel()

	const alterImports = "ALTER TABLE plugin_importer.imports ADD CONSTRAINT "
	const alterRows = "ALTER TABLE plugin_importer.import_rows ADD CONSTRAINT "
	tests := map[string]string{
		"the claim cannot be written":   alterImports + "no_committing CHECK (state <> 'committing')",
		"a row cannot be settled":       alterRows + "stays_pending CHECK (outcome = 'pending')",
		"the import cannot be finished": alterImports + "no_committed CHECK (state <> 'committed')",
	}

	for name, constraint := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p, pool, _, _ := newCommittingPlugin(t)
			id := uploadNamed(t, p, "broken.csv", "Name,Email\nMaria Perez,maria@example.com\n")
			mapNameAndEmail(t, p, id)
			if _, err := pool.Exec(t.Context(), constraint); err != nil {
				t.Fatalf("adding the constraint: %v", err)
			}

			_, err := commitImport(t, p, id)

			if err == nil {
				t.Fatal("ImportCommit() error = nil, want the refused write")
			}
		})
	}
}

func TestCommitWorksWithoutAPublisher(t *testing.T) {
	t.Parallel()

	cfg := newTestDatabase(t)
	contacts := newDirectory()
	p, err := importer.Register(sdk.Deps{DatabaseURL: cfg.URL(), Contacts: contacts})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	contacts.pool = newAssertionPool(t, cfg.URL())
	id := uploadNamed(t, p, "quiet.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)

	mustCommit(t, p, id)
}

func TestCommitReportsARowReadFailure(t *testing.T) {
	t.Parallel()

	p, pool, _, _ := newCommittingPlugin(t)
	id := uploadNamed(t, p, "broken.csv", "Name,Email\nMaria Perez,maria@example.com\n")
	mapNameAndEmail(t, p, id)
	if _, err := pool.Exec(t.Context(), "DROP TABLE plugin_importer.import_rows"); err != nil {
		t.Fatalf("dropping the rows table: %v", err)
	}

	_, err := commitImport(t, p, id)

	if err == nil {
		t.Fatal("ImportCommit() error = nil, want the row read failure")
	}
}
