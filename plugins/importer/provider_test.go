// SPDX-License-Identifier: Elastic-2.0

package importer_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/plugins/importer"
	"github.com/gopherium/alphone/sdk"
)

var _ sdk.FieldConsumer = (*importer.Plugin)(nil)

var errProvider = errors.New("the field catalogue is unreachable")

// fieldProvider answers the importer with a settable set of live fields.
type fieldProvider struct {
	mu      sync.Mutex
	fields  []sdk.ContactField
	listErr error
	refuse  map[string]string
	written map[uuid.UUID]map[string]string
	writes  int
}

// newFieldProvider returns a provider serving one date field.
func newFieldProvider(fields ...sdk.ContactField) *fieldProvider {
	return &fieldProvider{
		fields:  fields,
		refuse:  map[string]string{},
		written: map[uuid.UUID]map[string]string{},
	}
}

// LiveContactFields lists the settable fields.
func (f *fieldProvider) LiveContactFields(context.Context) ([]sdk.ContactField, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.fields, nil
}

// CheckContactFieldTexts refuses a name the provider does not serve or a settable text.
func (f *fieldProvider) CheckContactFieldTexts(_ context.Context, values map[string]string) error {
	for name, text := range values {
		if !f.holds(name) {
			return fmt.Errorf("%w: no live field holds %s", sdk.ErrInvalidFieldText, name)
		}
		if kind, refused := f.refuse[text]; refused {
			return fmt.Errorf("%w: %s expects %s", sdk.ErrInvalidFieldText, name, kind)
		}
	}
	return nil
}

// WriteContactFieldTexts records the texts written onto one contact.
func (f *fieldProvider) WriteContactFieldTexts(
	ctx context.Context, contactID uuid.UUID, values map[string]string,
) error {
	if err := f.CheckContactFieldTexts(ctx, values); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes++
	held := f.written[contactID]
	if held == nil {
		held = map[string]string{}
		f.written[contactID] = held
	}
	for name, text := range values {
		held[name] = text
	}
	return nil
}

// holds reports whether the provider serves the named field.
func (f *fieldProvider) holds(name string) bool {
	for _, field := range f.fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

// birthDate is the field every import mapping scenario maps its third column onto.
func birthDate() sdk.ContactField {
	return sdk.ContactField{Name: "birthDate", Label: "Birth date"}
}

// newServedPlugin returns a committing plugin wired to the given providers.
func newServedPlugin(t *testing.T, providers ...sdk.FieldProvider) (
	*importer.Plugin, *pgxpool.Pool, *directory,
) {
	t.Helper()
	p, pool, contacts, _ := newCommittingPlugin(t)
	p.UseFieldProviders(providers)
	return p, pool, contacts
}

// mapNameEmailAndField assigns the three columns of the field scenarios.
func mapNameEmailAndField(t *testing.T, p *importer.Plugin, id uuid.UUID, field string) error {
	t.Helper()
	_, err := p.MutationResolvers().ImportSetMapping(t.Context(), id, []*model.ImportAssignmentInput{
		{Column: 0, Field: "name"},
		{Column: 1, Field: "email"},
		{Column: 2, Field: field},
	})
	return err
}

// fieldCSV holds one header row and the given row beneath it.
func fieldCSV(row string) string {
	return "Name,Email,Birth date\n" + row + "\n"
}

// byName returns the id of the contact the directory stored under a name.
func (d *directory) byName(t *testing.T, name string) uuid.UUID {
	t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, held := range d.contacts {
		if held.Name == name {
			return id
		}
	}
	t.Fatalf("the directory holds no contact named %q", name)
	return uuid.Nil
}

// stateOf returns the state one import holds.
func stateOf(t *testing.T, pool *pgxpool.Pool, importID uuid.UUID) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(t.Context(),
		"SELECT state FROM plugin_importer.imports WHERE id = $1", importID).Scan(&state); err != nil {
		t.Fatalf("reading the import state: %v", err)
	}
	return state
}

// reasonOf returns the reason one staged row settled with.
func reasonOf(t *testing.T, pool *pgxpool.Pool, importID uuid.UUID, position int) string {
	t.Helper()
	var reason *string
	if err := pool.QueryRow(t.Context(),
		"SELECT reason FROM plugin_importer.import_rows WHERE import_id = $1 AND position = $2",
		importID, position).Scan(&reason); err != nil {
		t.Fatalf("reading the row reason: %v", err)
	}
	if reason == nil {
		return ""
	}
	return *reason
}

// registryNames lists the names the mappable registry answers with, in order.
func registryNames(t *testing.T, p *importer.Plugin) []string {
	t.Helper()
	listed, err := p.QueryResolvers().ImportFields(t.Context())
	if err != nil {
		t.Fatalf("ImportFields() error = %v, want nil", err)
	}
	names := make([]string, len(listed))
	for i, field := range listed {
		names[i] = field.Name
	}
	return names
}

func TestImportFieldsListsProviderFieldsAfterTheCoreColumns(t *testing.T) {
	t.Parallel()

	p, _, _ := newServedPlugin(t, newFieldProvider(birthDate()))

	listed, err := p.QueryResolvers().ImportFields(t.Context())

	if err != nil {
		t.Fatalf("ImportFields() error = %v, want nil", err)
	}
	want := []string{"name", "email", "phone", "birthDate"}
	if len(listed) != len(want) {
		t.Fatalf("listed = %d fields, want %d", len(listed), len(want))
	}
	for i, field := range listed {
		if field.Name != want[i] {
			t.Errorf("field %d = %q, want %q", i, field.Name, want[i])
		}
	}
	if listed[3].Label != "Birth date" || listed[3].Required {
		t.Errorf("the served field = %+v, want its label and no requirement", listed[3])
	}
}

func TestImportFieldsServesTheCoreColumnsWithoutAProvider(t *testing.T) {
	t.Parallel()

	p, _, _ := newServedPlugin(t)

	if got := registryNames(t, p); strings.Join(got, ",") != "name,email,phone" {
		t.Errorf("names = %v, want the core columns alone", got)
	}
}

func TestImportFieldsLeavesOutAFieldNamedAfterACoreColumn(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(sdk.ContactField{Name: "email", Label: "Second email"})
	p, _, _ := newServedPlugin(t, provider)

	names := registryNames(t, p)

	var seen int
	for _, name := range names {
		if name == "email" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("names = %v, want email listed exactly once", names)
	}
	if strings.Join(names, ",") != "name,email,phone" {
		t.Errorf("names = %v, want the core columns alone", names)
	}
}

func TestImportFieldsKeepsTheFirstProviderToClaimAName(t *testing.T) {
	t.Parallel()

	first := newFieldProvider(sdk.ContactField{Name: "birthDate", Label: "Birth date"})
	second := newFieldProvider(sdk.ContactField{Name: "birthDate", Label: "Date of birth"})
	p, _, _ := newServedPlugin(t, first, second)

	listed, err := p.QueryResolvers().ImportFields(t.Context())

	if err != nil {
		t.Fatalf("ImportFields() error = %v, want nil", err)
	}
	if len(listed) != 4 {
		t.Fatalf("listed = %d fields, want the name claimed once", len(listed))
	}
	if listed[3].Label != "Birth date" {
		t.Errorf("label = %q, want the first provider to win", listed[3].Label)
	}
}

func TestImportFieldsReportsAProviderFailure(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	provider.listErr = errProvider
	p, _, _ := newServedPlugin(t, provider)

	if _, err := p.QueryResolvers().ImportFields(t.Context()); !errors.Is(err, errProvider) {
		t.Errorf("error = %v, want the outage reported rather than a shrunken registry", err)
	}
}

func TestSetMappingAcceptsAProviderServedField(t *testing.T) {
	t.Parallel()

	p, _, _ := newServedPlugin(t, newFieldProvider(birthDate()))
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,1990-04-17"))

	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Errorf("ImportSetMapping() error = %v, want the served field accepted", err)
	}
}

func TestSetMappingRefusesAFieldNoProviderServes(t *testing.T) {
	t.Parallel()

	p, _, _ := newServedPlugin(t, newFieldProvider(birthDate()))
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,1990-04-17"))

	err := mapNameEmailAndField(t, p, id, "neverDefined")

	if got := refusalCode(t, err); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
}

func TestCommitWritesAMappedCellIntoItsField(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, _, contacts := newServedPlugin(t, provider)
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,1990-04-17"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}

	committed := mustCommit(t, p, id)

	if committed.Imported != 1 {
		t.Fatalf("imported = %d, want 1", committed.Imported)
	}
	created := contacts.byName(t, "Maria Perez")
	if provider.written[created]["birthDate"] != "1990-04-17" {
		t.Errorf("written = %v, want the cell stored on the created contact", provider.written)
	}
}

func TestCommitLeavesAnEmptyCellUnwritten(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, _, _ := newServedPlugin(t, provider)
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}

	committed := mustCommit(t, p, id)

	if committed.Imported != 1 {
		t.Fatalf("imported = %d, want 1", committed.Imported)
	}
	if provider.writes != 0 {
		t.Errorf("writes = %d, want an empty cell to reach no provider", provider.writes)
	}
}

func TestCommitFailsARowWhoseFieldTextIsRefused(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	provider.refuse["not a date"] = "DATE"
	p, pool, contacts := newServedPlugin(t, provider)
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,not a date"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}

	committed := mustCommit(t, p, id)

	if committed.Failed != 1 || committed.Imported != 0 {
		t.Fatalf("counts = %d failed, %d imported, want 1 and 0", committed.Failed, committed.Imported)
	}
	if contacts.creates != 0 {
		t.Errorf("creates = %d, want the contact never created", contacts.creates)
	}
	reason := reasonOf(t, pool, id, 1)
	for _, want := range []string{"birthDate", "DATE"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason = %q, want it to name %q", reason, want)
		}
	}
}

func TestCommitWritesNothingForASkippedRow(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, _, contacts := newServedPlugin(t, provider)
	contacts.seed("Maria Perez", "maria@example.com")
	id := uploadNamed(t, p, "leads.csv", fieldCSV("M. Perez,maria@example.com,1990-04-17"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}

	committed := mustCommit(t, p, id)

	if committed.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", committed.Skipped)
	}
	if provider.writes != 0 {
		t.Errorf("writes = %d, want an import to create rather than update", provider.writes)
	}
}

func TestCommitWritesNothingWhenTheCreateLosesItsRace(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, _, contacts := newServedPlugin(t, provider)
	contacts.seed("Maria Perez", "maria@example.com")
	contacts.hideClaims = true
	id := uploadNamed(t, p, "leads.csv", fieldCSV("M. Perez,maria@example.com,1990-04-17"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}

	committed := mustCommit(t, p, id)

	if committed.Skipped != 1 {
		t.Fatalf("skipped = %d, want the lost race skipped", committed.Skipped)
	}
	if provider.writes != 0 {
		t.Errorf("writes = %d, want nothing written onto the contact that already existed", provider.writes)
	}
}

func TestCommitRefusesAMappingNamingAVanishedField(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, pool, _ := newServedPlugin(t, provider)
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,1990-04-17"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}
	provider.fields = nil

	_, err := commitImport(t, p, id)

	if got := refusalCode(t, err); got != "VALIDATION" {
		t.Fatalf("code = %q, want VALIDATION", got)
	}
	if !strings.Contains(err.Error(), "birthDate") {
		t.Errorf("error = %v, want it to name the vanished field", err)
	}
	if got := stateOf(t, pool, id); got != "ready" {
		t.Errorf("state = %q, want the import left editable", got)
	}
}

func TestCommitReportsAProviderOutageWithoutClaiming(t *testing.T) {
	t.Parallel()

	provider := newFieldProvider(birthDate())
	p, pool, _ := newServedPlugin(t, provider)
	id := uploadNamed(t, p, "leads.csv", fieldCSV("Maria Perez,maria@example.com,1990-04-17"))
	if err := mapNameEmailAndField(t, p, id, "birthDate"); err != nil {
		t.Fatalf("ImportSetMapping() error = %v, want nil", err)
	}
	provider.listErr = errProvider

	if _, err := commitImport(t, p, id); !errors.Is(err, errProvider) {
		t.Fatalf("error = %v, want the outage reported", err)
	}
	if got := stateOf(t, pool, id); got != "ready" {
		t.Errorf("state = %q, want the import left editable", got)
	}
}
