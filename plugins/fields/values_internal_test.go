// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/sdk"
)

func TestCoerceRefusesAnUnknownKind(t *testing.T) {
	t.Parallel()

	if _, err := coerce(kind("TIMESTAMP"), "x"); !errors.Is(err, errWrongKind) {
		t.Errorf("error = %v, want errWrongKind for a kind no definition may declare", err)
	}
}

func TestCoerceRefusesADateThatIsNotText(t *testing.T) {
	t.Parallel()

	if _, err := coerce(kindDate, true); !errors.Is(err, errWrongKind) {
		t.Errorf("error = %v, want errWrongKind", err)
	}
}

func TestValueResolversReportAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)
	scoped := sdk.WithRequestScope(t.Context(), sdk.NewRequestScope())
	contactID := uuid.Must(uuid.NewV7())

	if _, err := p.loadValues(scoped, contactID); err == nil {
		t.Error("loadValues() error = nil, want the closed pool reported")
	}
	obj := &model.Contact{ID: contactID}
	if _, err := (ContactResolvers{plugin: p}).Field(scoped, obj, "birthDate"); err == nil {
		t.Error("Field() error = nil, want the closed pool reported")
	}
}

func TestFieldNeedsARequestScope(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	_, err := p.loadValues(t.Context(), uuid.Must(uuid.NewV7()))

	if err == nil {
		t.Error("loadValues() error = nil, want the missing request scope reported")
	}
}

func TestWriteContactFieldsReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)
	if err := p.store.define(t.Context(), defined(t, "birthDate", "DATE")); err != nil {
		t.Fatalf("define() error = %v, want nil", err)
	}
	if err := p.catalog.reload(t.Context()); err != nil {
		t.Fatalf("reload() error = %v, want nil", err)
	}

	_, err := (MutationResolvers{plugin: p}).WriteContactFields(
		t.Context(), uuid.Must(uuid.NewV7()), map[string]any{"birthDate": "1990-04-17"})

	if err == nil {
		t.Error("WriteContactFields() error = nil, want the unknown contact refused")
	}
}

func TestClearValuesReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	err := p.store.clearValues(t.Context(), uuid.Must(uuid.NewV7()), []string{"birthDate"})

	if err == nil {
		t.Error("clearValues() error = nil, want the closed pool reported")
	}
}

func TestRefusalForReportsAClosedPool(t *testing.T) {
	t.Parallel()

	p := newClosedPlugin(t)

	err := p.store.refusalFor(t.Context(), defined(t, "birthDate", "DATE"))

	if err == nil {
		t.Error("refusalFor() error = nil, want the closed pool reported")
	}
}

func TestValuesForReportsRowsItCannotRead(t *testing.T) {
	t.Parallel()

	p := newMigratedPlugin(t)

	_, err := p.store.collectValues(t.Context(), "SELECT 1 WHERE $1::uuid[] IS NOT NULL",
		[]uuid.UUID{uuid.Must(uuid.NewV7())})

	if err == nil {
		t.Fatal("collectValues() error = nil, want the unreadable row reported")
	}
	if !strings.Contains(err.Error(), "one contact's values") {
		t.Errorf("error = %v, want it to name the read", err)
	}
}
