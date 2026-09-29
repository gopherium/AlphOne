// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/cucumber/godog"
)

// orderFieldsMutation sets the order of every live field through the graph.
const orderFieldsMutation = `mutation($ids: [UUID!]!) { orderFields(ids: $ids) }`

// tableColumn returns the cells of a one column table under its header row.
func tableColumn(table *godog.Table) []string {
	cells := make([]string, 0, len(table.Rows)-1)
	for _, row := range table.Rows[1:] {
		cells = append(cells, row.Cells[0].Value)
	}
	return cells
}

// catalogueIDs maps every field name the catalogue behind a secret holds, archived ones included, to its id.
func (w *world) catalogueIDs(ctx context.Context, secret string) (map[string]string, error) {
	listed, err := w.operationAs(ctx, secret, fieldsQuery, map[string]any{"includeArchived": true})
	if err != nil {
		return nil, err
	}
	if len(listed.Errors) > 0 {
		return nil, fmt.Errorf("the catalogue was refused, answered %s", w.answered)
	}
	ids := make(map[string]string, len(listed.Data.Fields))
	for _, held := range listed.Data.Fields {
		ids[held.Name] = held.ID
	}
	return ids, nil
}

// fieldIDs resolves names through the caller's catalogue, then the owner's, so a scenario can name a foreign field.
func (w *world) fieldIDs(ctx context.Context, secret string, names []string) ([]string, error) {
	own, err := w.catalogueIDs(ctx, secret)
	if err != nil {
		return nil, err
	}
	owner, err := w.catalogueIDs(ctx, w.secret)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		id := cmp.Or(own[name], owner[name])
		if id == "" {
			return nil, fmt.Errorf("no catalogue holds a field named %q", name)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// fieldNames returns the names the owner's catalogue lists, in the order it lists them.
func (w *world) fieldNames(ctx context.Context) ([]string, error) {
	listed, err := w.operation(ctx, fieldsQuery, map[string]any{})
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(listed.Data.Fields))
	for _, held := range listed.Data.Fields {
		names = append(names, held.Name)
	}
	return names, nil
}

// bindFieldOrderSteps binds the steps that order the fields and read the order back.
func bindFieldOrderSteps(sc *godog.ScenarioContext) {
	sc.Step(`^(?:the operator|the tenant "([^"]*)") orders the fields:$`,
		func(ctx context.Context, tenant string, table *godog.Table) error {
			w := worldFrom(ctx)
			secret, err := w.secretFor(tenant)
			if err != nil {
				return err
			}
			ids, err := w.fieldIDs(ctx, secret, tableColumn(table))
			if err != nil {
				return err
			}
			_, err = w.operationAs(ctx, secret, orderFieldsMutation, map[string]any{"ids": ids})
			return err
		})

	sc.Then(`^the order is accepted$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var answer graphAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		if len(answer.Errors) > 0 || !answer.Data.OrderFields {
			return fmt.Errorf("the graph did not accept the order, answered %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the catalogue lists the fields in order:$`, func(ctx context.Context, table *godog.Table) error {
		w := worldFrom(ctx)
		names, err := w.fieldNames(ctx)
		if err != nil {
			return err
		}
		if want := tableColumn(table); !slices.Equal(names, want) {
			return fmt.Errorf("fields = %v, want %v, answered %s", names, want, w.answered)
		}
		return nil
	})
}
