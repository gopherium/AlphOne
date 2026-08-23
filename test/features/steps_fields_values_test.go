// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// writeFieldsMutation writes one contact's field values through the graph.
const writeFieldsMutation = `mutation($contactId: UUID!, $values: JSON!) {
	writeContactFields(contactId: $contactId, values: $values)
}`

// contactFieldQuery reads one runtime defined field by its own name.
const contactFieldQuery = `query($id: UUID!) { contact(id: $id) { name %s } }`

// fieldAnswer is the envelope a runtime field read returns.
type fieldAnswer struct {
	Data struct {
		Contact map[string]any `json:"contact"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// readField asks the graph for one runtime defined field of the seeded contact.
func (w *world) readField(ctx context.Context, name string) (fieldAnswer, error) {
	document := fmt.Sprintf(contactFieldQuery, name)
	body, err := json.Marshal(map[string]any{
		"query":     document,
		"variables": map[string]any{"id": w.lastContact.String()},
	})
	if err != nil {
		return fieldAnswer{}, fmt.Errorf("encoding the read: %w", err)
	}
	raw, err := w.postGraph(ctx, string(body))
	if err != nil {
		return fieldAnswer{}, err
	}
	w.answered = raw
	var answer fieldAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return fieldAnswer{}, fmt.Errorf("decoding %s: %w", raw, err)
	}
	return answer, nil
}

// registerFieldsValuesSteps binds the field value steps and the world lifecycle.
func registerFieldsValuesSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerFieldsCatalogSteps(sc, t)

	sc.Given(`^a contact named "([^"]*)"$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).seedContact(ctx, name)
		return err
	})

	sc.Step(`^the operator writes "([^"]*)" into "([^"]*)" of the contact$`,
		func(ctx context.Context, value, name string) error {
			w := worldFrom(ctx)
			_, err := w.operation(ctx, writeFieldsMutation, map[string]any{
				"contactId": w.lastContact.String(),
				"values":    map[string]any{name: value},
			})
			return err
		})

	sc.When(`^the contact is queried for the field "([^"]*)"$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).readField(ctx, name)
		return err
	})

	sc.Then(`^querying the contact for "([^"]*)" answers "([^"]*)"$`,
		func(ctx context.Context, name, want string) error {
			answered, err := worldFrom(ctx).readField(ctx, name)
			if err != nil {
				return err
			}
			if len(answered.Errors) > 0 {
				return fmt.Errorf("the graph refused the read, answered %s", worldFrom(ctx).answered)
			}
			if got := answered.Data.Contact[name]; got != want {
				return fmt.Errorf("%s = %#v, want %q", name, got, want)
			}
			return nil
		})

	sc.Then(`^querying the contact for "([^"]*)" is refused as an unknown field$`,
		func(ctx context.Context, name string) error {
			if _, err := worldFrom(ctx).readField(ctx, name); err != nil {
				return err
			}
			return worldFrom(ctx).refusedAsUnknownField()
		})

	sc.Then(`^the graph refuses the query as an unknown field$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedAsUnknownField()
	})

	sc.Then(`^the write is refused naming "([^"]*)" as the bad key$`,
		func(ctx context.Context, name string) error {
			w := worldFrom(ctx)
			if err := w.refusedFor("VALIDATION"); err != nil {
				return err
			}
			if !strings.Contains(string(w.answered), name) {
				return fmt.Errorf("the error does not name %q, answered %s", name, w.answered)
			}
			return nil
		})

	sc.Then(`^the write is refused for a value of the wrong kind$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.refusedFor("VALIDATION"); err != nil {
			return err
		}
		if !strings.Contains(string(w.answered), "does not match the kind") {
			return fmt.Errorf("the error does not name the kind, answered %s", w.answered)
		}
		return nil
	})
}

// refusedAsUnknownField reports whether the last answer refused an unknown field.
func (w *world) refusedAsUnknownField() error {
	if !strings.Contains(string(w.answered), "Cannot query field") {
		return fmt.Errorf("the graph did not refuse the field, answered %s", w.answered)
	}
	return nil
}
