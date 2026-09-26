// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

func TestDefineFieldNamesTheReasonItRefuses(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	_, err := (MutationResolvers{plugin: p}).DefineField(t.Context(), "Not Camel", "Label", model.FieldKindDate, nil)

	var raised sdk.GraphError
	if !errors.As(err, &raised) || raised.Reason != "field_name_malformed" {
		t.Errorf("error = %v, want the malformed name named as a reason", err)
	}
}

func TestDefineFieldNamesALabelBeyondTheCap(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)
	label := strings.Repeat("x", labelMax+1)

	_, err := (MutationResolvers{plugin: p}).DefineField(t.Context(), "birthDate", label, model.FieldKindDate, nil)

	var raised sdk.GraphError
	if !errors.As(err, &raised) || raised.Reason != "field_label_too_long" {
		t.Errorf("error = %v, want the long label named as a reason", err)
	}
}

func TestDefineFieldStoresTheSubFieldsItWasGiven(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	given := []*model.FieldSubFieldInput{
		{Name: "date", Label: "Date", Kind: model.FieldKindDate},
		{Name: "comment", Label: " Comment ", Kind: model.FieldKindLongtext},
	}

	defined, err := (MutationResolvers{plugin: p}).DefineField(
		t.Context(), "history", "History", model.FieldKindRepeater, given)
	if err != nil {
		t.Fatalf("DefineField() error = %v, want nil", err)
	}
	listed, err := (QueryResolvers{plugin: p}).Fields(t.Context(), nil)
	if err != nil {
		t.Fatalf("Fields() error = %v, want nil", err)
	}

	want := []*model.FieldSubField{
		{Name: "date", Label: "Date", Kind: model.FieldKindDate},
		{Name: "comment", Label: "Comment", Kind: model.FieldKindLongtext},
	}
	if !reflect.DeepEqual(defined.SubFields, want) {
		t.Errorf("defined sub fields = %+v, want %+v", defined.SubFields, want)
	}
	if len(listed) != 1 || !reflect.DeepEqual(listed[0].SubFields, want) {
		t.Errorf("listed = %+v, want the repeater listed with its sub fields", listed)
	}
}

func TestEverySubFieldSentinelNamesItsReason(t *testing.T) {
	t.Parallel()

	want := map[error]string{
		errSubFieldsRequired:   "field_sub_fields_required",
		errSubFieldsUnexpected: "field_sub_fields_unexpected",
		errSubFieldNested:      "field_sub_field_nested",
		errSubFieldNameInvalid: "field_sub_field_name_invalid",
		errSubFieldNameTaken:   "field_sub_field_name_taken",
	}

	for sentinel, reason := range want {
		if got := fieldReason(sentinel); got != reason {
			t.Errorf("fieldReason(%v) = %q, want %q", sentinel, got, reason)
		}
	}
}

func TestEveryReasonHasAFrontendMessage(t *testing.T) {
	t.Parallel()

	templates, err := os.ReadFile(filepath.Join("frontend", "errorTemplates.ts"))
	if err != nil {
		t.Fatalf("reading the frontend templates: %v", err)
	}

	for _, held := range fieldReasons {
		if !strings.Contains(string(templates), "\t"+held.reason+": __(") {
			t.Errorf("reason %q has no frontend message, want the screens to speak it", held.reason)
		}
	}
}

func TestFieldReasonAnswersNothingForAnUnlistedError(t *testing.T) {
	t.Parallel()

	if got := fieldReason(errCatalogue); got != "" {
		t.Errorf("fieldReason() = %q, want nothing for an error outside the table", got)
	}
}

// errCatalogue is the failure a wedged catalogue read reports.
var errCatalogue = errors.New("catalogue unavailable")

// newClosedPlugin returns a migrated plugin whose pool is already closed.
func newClosedPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	p.pool.Close()
	return p
}

// newWedgedPlugin returns a working store whose catalogue always fails to read.
func newWedgedPlugin(t *testing.T) *Plugin {
	t.Helper()
	p := newMigratedPlugin(t)
	loader := newFakeLoader()
	loader.err = errCatalogue
	p.catalog = newCatalog(loader, 0, 0)
	return p
}

func TestStoreReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err == nil {
		t.Error("create() error = nil, want the closed pool reported")
	}
	if err := p.store.archive(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("archive() error = nil, want the closed pool reported")
	}
	if _, err := p.store.liveDefinitions(t.Context()); err == nil {
		t.Error("liveDefinitions() error = nil, want the closed pool reported")
	}
}

func TestResolversReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	if _, err := (QueryResolvers{plugin: p}).Fields(t.Context(), nil); err == nil {
		t.Error("Fields() error = nil, want the closed pool reported")
	}
	_, err := (MutationResolvers{plugin: p}).DefineField(t.Context(), "birthDate", "Birth date", model.FieldKindDate, nil)
	if err == nil {
		t.Error("DefineField() error = nil, want the closed pool reported")
	}
	if _, err := (MutationResolvers{plugin: p}).ArchiveField(t.Context(), uuid.Must(uuid.NewV7())); err == nil {
		t.Error("ArchiveField() error = nil, want the closed pool reported")
	}
}

func TestAWriteReportsAFailedCatalogueRead(t *testing.T) {
	t.Parallel()

	p := newWedgedPlugin(t)

	_, err := (MutationResolvers{plugin: p}).WriteContactFields(
		t.Context(), uuid.Must(uuid.NewV7()), map[string]any{"birthDate": "1990-04-17"})

	if !errors.Is(err, errCatalogue) {
		t.Errorf("WriteContactFields() error = %v, want the catalogue failure", err)
	}
}

func TestADefineOrArchiveTheStoreCommittedAnswersSuccess(t *testing.T) {
	t.Parallel()

	p := newWedgedPlugin(t)

	defined, err := (MutationResolvers{plugin: p}).DefineField(
		t.Context(), "loyaltyPoints", "Points", model.FieldKindNumber, nil)
	if err != nil {
		t.Fatalf("DefineField() error = %v, want the committed define answered", err)
	}
	archived, err := (MutationResolvers{plugin: p}).ArchiveField(t.Context(), defined.ID)

	if err != nil || !archived {
		t.Errorf("ArchiveField() = %v, %v, want the committed archive answered", archived, err)
	}
}

func TestAWriteChecksTheCallersOwnFields(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	acme := inTenant(t, p)
	contactID := seedContact(t, p, "Maria Perez")
	definedField(t, p, t.Context(), "birthDate")
	definedField(t, p, acme, "shoeSize")
	resolvers := MutationResolvers{plugin: p}

	if _, err := resolvers.WriteContactFields(acme, contactID, map[string]any{"shoeSize": "44"}); err != nil {
		t.Errorf("Acme writing its own shoeSize error = %v, want nil", err)
	}
	if _, err := resolvers.WriteContactFields(t.Context(), contactID, map[string]any{"shoeSize": "44"}); err == nil {
		t.Error("the default tenant writing Acme's shoeSize error = nil, want it refused")
	}
	if _, err := resolvers.WriteContactFields(acme, contactID, map[string]any{"birthDate": "x"}); err == nil {
		t.Error("Acme writing the default tenant's birthDate error = nil, want it refused")
	}
}

func TestDefiningAndArchivingRenewTheCallersFields(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	contactID := seedContact(t, p, "Maria Perez")
	resolvers := MutationResolvers{plugin: p}
	mustView(t, p.catalog, t.Context())

	stored, err := resolvers.DefineField(t.Context(), "birthDate", "Birth date", model.FieldKindText, nil)
	if err != nil {
		t.Fatalf("DefineField() error = %v, want nil", err)
	}
	if _, err := resolvers.WriteContactFields(t.Context(), contactID, map[string]any{"birthDate": "x"}); err != nil {
		t.Fatalf("a write after the define error = %v, want the new field known", err)
	}
	if _, err := resolvers.ArchiveField(t.Context(), stored.ID); err != nil {
		t.Fatalf("ArchiveField() error = %v, want nil", err)
	}

	if _, err := resolvers.WriteContactFields(t.Context(), contactID, map[string]any{"birthDate": "x"}); err == nil {
		t.Error("a write after the archive error = nil, want the archived field refused")
	}
}

func TestFieldsListsArchivedDefinitionsOnRequest(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	stored := defined(t, "birthDate", "DATE")
	if err := p.store.define(t.Context(), stored); err != nil {
		t.Fatalf("create() error = %v, want nil", err)
	}
	if err := p.store.archive(t.Context(), stored.ID); err != nil {
		t.Fatalf("archive() error = %v, want nil", err)
	}
	wanted := true

	listed, err := (QueryResolvers{plugin: p}).Fields(t.Context(), &wanted)

	if err != nil {
		t.Fatalf("Fields() error = %v, want nil", err)
	}
	if len(listed) != 1 || listed[0].ArchivedAt == nil {
		t.Errorf("fields = %+v, want the archived definition listed", listed)
	}
}
