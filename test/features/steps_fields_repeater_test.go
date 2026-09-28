// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// defineWithSubFieldsMutation declares one field and its sub fields through the graph.
const defineWithSubFieldsMutation = `mutation($name: String!, $label: String!, $kind: FieldKind!,
	$subFields: [FieldSubFieldInput!]) {
	defineField(name: $name, label: $label, kind: $kind, subFields: $subFields) { id name label kind }
}`

// subFieldsQuery reads every live field with its sub fields through the graph.
const subFieldsQuery = `{ fields { name subFields { name label kind } } }`

// tableRecords turns a table under a header row into one record per row, leaving empty cells out.
func tableRecords(table *godog.Table) []map[string]any {
	header := table.Rows[0].Cells
	records := make([]map[string]any, 0, len(table.Rows)-1)
	for _, row := range table.Rows[1:] {
		record := map[string]any{}
		for at, cell := range row.Cells {
			if cell.Value != "" {
				record[header[at].Value] = cell.Value
			}
		}
		records = append(records, record)
	}
	return records
}

// defineWithSubFields declares a field holding the given sub fields, remembering the id when the graph accepts it.
func (w *world) defineWithSubFields(
	ctx context.Context, name, label, kind string, subFields []map[string]any,
) error {
	return w.defineWithSubFieldsAs(ctx, w.secret, name, label, kind, subFields)
}

// defineWithSubFieldsAs declares a field holding the given sub fields as the given secret.
func (w *world) defineWithSubFieldsAs(
	ctx context.Context, secret, name, label, kind string, subFields []map[string]any,
) error {
	answer, err := w.operationAs(ctx, secret, defineWithSubFieldsMutation, map[string]any{
		"name": name, "label": label, "kind": kind, "subFields": subFields,
	})
	if err != nil {
		return err
	}
	if id, parseErr := uuid.Parse(answer.Data.DefineField.ID); parseErr == nil {
		w.lastField = id
	}
	return nil
}

// sameJSON reports whether two values encode to the same JSON.
func sameJSON(got, want any) (bool, error) {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false, fmt.Errorf("encoding %#v: %w", got, err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false, fmt.Errorf("encoding %#v: %w", want, err)
	}
	return string(gotJSON) == string(wantJSON), nil
}

// registerFieldsRepeaterSteps binds the repeater steps beside the catalogue, value, graph and import steps.
func registerFieldsRepeaterSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newImportWorld(t)), nil
	})
	bindFieldsCatalogSteps(sc)
	bindFieldValueSteps(sc)
	bindIntrospectionSteps(sc)
	bindImportSteps(sc)
	bindRepeaterDefinitionSteps(sc)
	bindRepeaterRowSteps(sc)
	bindRepeaterEntrySteps(sc)
}

// bindRepeaterDefinitionSteps binds the steps that define repeaters and read their sub fields.
func bindRepeaterDefinitionSteps(sc *godog.ScenarioContext) {
	sc.Step(`^(?:the operator defines |the tenant "([^"]*)" defines )?the repeater "([^"]*)" labelled "([^"]*)" `+
		`(?:is defined )?with sub fields:$`,
		func(ctx context.Context, tenant, name, label string, table *godog.Table) error {
			w := worldFrom(ctx)
			secret, err := w.secretFor(tenant)
			if err != nil {
				return err
			}
			return w.defineWithSubFieldsAs(ctx, secret, name, label, "REPEATER", tableRecords(table))
		})

	sc.When(`^the operator defines the repeater "([^"]*)" labelled "([^"]*)" with no sub fields$`,
		func(ctx context.Context, name, label string) error {
			return worldFrom(ctx).defineWithSubFields(ctx, name, label, "REPEATER", []map[string]any{})
		})

	sc.When(`^the operator defines the field "([^"]*)" labelled "([^"]*)" of kind ([A-Z]+) with sub fields:$`,
		func(ctx context.Context, name, label, kind string, table *godog.Table) error {
			return worldFrom(ctx).defineWithSubFields(ctx, name, label, kind, tableRecords(table))
		})

	sc.Then(`^the repeater "([^"]*)" lists the sub fields:$`,
		func(ctx context.Context, name string, table *godog.Table) error {
			w := worldFrom(ctx)
			listed, err := w.operation(ctx, subFieldsQuery, map[string]any{})
			if err != nil {
				return err
			}
			for _, held := range listed.Data.Fields {
				if held.Name != name {
					continue
				}
				same, err := sameJSON(held.SubFields, tableRecords(table))
				if err != nil || same {
					return err
				}
				return fmt.Errorf("sub fields = %+v, answered %s", held.SubFields, w.answered)
			}
			return fmt.Errorf("the catalogue lists no field named %q, answered %s", name, w.answered)
		})
}

// bindRepeaterRowSteps binds the step that writes a whole list of rows through the field values.
func bindRepeaterRowSteps(sc *godog.ScenarioContext) {
	sc.Step(`^the operator writes the rows into "([^"]*)" of the contact:$`,
		func(ctx context.Context, name string, table *godog.Table) error {
			return worldFrom(ctx).writeValue(ctx, name, tableRecords(table))
		})
}
