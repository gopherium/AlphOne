// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// defineFieldMutation declares one field through the graph.
const defineFieldMutation = `mutation($name: String!, $label: String!, $kind: FieldKind!) {
	defineField(name: $name, label: $label, kind: $kind) { id name label kind }
}`

// archiveFieldMutation hides one field through the graph.
const archiveFieldMutation = `mutation($id: UUID!) { archiveField(id: $id) }`

// fieldsQuery reads the catalogue through the graph.
const fieldsQuery = `query($includeArchived: Boolean) {
	fields(includeArchived: $includeArchived) { id name label kind archivedAt }
}`

// graphAnswer is the envelope every catalogue step reads.
type graphAnswer struct {
	Data struct {
		DefineField struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Label string `json:"label"`
			Kind  string `json:"kind"`
		} `json:"defineField"`
		Fields []struct {
			ID         string  `json:"id"`
			Name       string  `json:"name"`
			Label      string  `json:"label"`
			Kind       string  `json:"kind"`
			ArchivedAt *string `json:"archivedAt"`
		} `json:"fields"`
	} `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// operation posts a graph operation with variables and keeps the raw answer.
func (w *world) operation(ctx context.Context, document string, variables map[string]any) (graphAnswer, error) {
	body, err := json.Marshal(map[string]any{"query": document, "variables": variables})
	if err != nil {
		return graphAnswer{}, fmt.Errorf("encoding the operation: %w", err)
	}
	raw, err := w.postGraph(ctx, string(body))
	if err != nil {
		return graphAnswer{}, err
	}
	w.answered = raw
	var answer graphAnswer
	if err := json.Unmarshal(raw, &answer); err != nil {
		return graphAnswer{}, fmt.Errorf("decoding %s: %w", raw, err)
	}
	return answer, nil
}

// defineField declares a field, remembering the id when the graph accepts it.
func (w *world) defineField(ctx context.Context, name, label, kind string) error {
	answer, err := w.operation(ctx, defineFieldMutation,
		map[string]any{"name": name, "label": label, "kind": kind})
	if err != nil {
		return err
	}
	if id, parseErr := uuid.Parse(answer.Data.DefineField.ID); parseErr == nil {
		w.lastField = id
	}
	return nil
}

// refusedFor reports whether the last answer carries the given extensions code.
func (w *world) refusedFor(code string) error {
	var answer graphAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if len(answer.Errors) == 0 {
		return fmt.Errorf("the graph accepted the operation, answered %s", w.answered)
	}
	if got := answer.Errors[0].Extensions["code"]; got != code {
		return fmt.Errorf("code = %v, want %s, answered %s", got, code, w.answered)
	}
	return nil
}

// registerFieldsCatalogSteps binds the catalogue steps and the world lifecycle.
func registerFieldsCatalogSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})
	bindFieldsCatalogSteps(sc)
}

// bindFieldsCatalogSteps binds the catalogue steps onto an already booted world.
func bindFieldsCatalogSteps(sc *godog.ScenarioContext) {
	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.Step(`^the (?:operator defines the field|field) "([^"]*)" labelled "([^"]*)" of kind ([A-Z]+)(?: is defined)?$`,
		func(ctx context.Context, name, label, kind string) error {
			return worldFrom(ctx).defineField(ctx, name, label, kind)
		})

	sc.When(`^the operator archives the field "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		listed, err := w.operation(ctx, fieldsQuery, map[string]any{})
		if err != nil {
			return err
		}
		for _, held := range listed.Data.Fields {
			if held.Name != name {
				continue
			}
			_, err := w.operation(ctx, archiveFieldMutation, map[string]any{"id": held.ID})
			return err
		}
		return fmt.Errorf("the catalogue lists no field named %q", name)
	})

	sc.Then(`^the catalogue lists "([^"]*)" with label "([^"]*)" and kind ([A-Z]+)$`,
		func(ctx context.Context, name, label, kind string) error {
			listed, err := worldFrom(ctx).operation(ctx, fieldsQuery, map[string]any{})
			if err != nil {
				return err
			}
			for _, held := range listed.Data.Fields {
				if held.Name != name {
					continue
				}
				if held.Label != label || held.Kind != kind {
					return fmt.Errorf("field = %+v, want label %q and kind %s", held, label, kind)
				}
				return nil
			}
			return fmt.Errorf("the catalogue lists no field named %q", name)
		})

	sc.Then(`^the catalogue does not list "([^"]*)"$`, func(ctx context.Context, name string) error {
		listed, err := worldFrom(ctx).operation(ctx, fieldsQuery, map[string]any{})
		if err != nil {
			return err
		}
		for _, held := range listed.Data.Fields {
			if held.Name == name {
				return fmt.Errorf("the catalogue still lists %q", name)
			}
		}
		return nil
	})

	sc.Then(`^the catalogue lists "([^"]*)" among archived definitions$`,
		func(ctx context.Context, name string) error {
			w := worldFrom(ctx)
			listed, err := w.operation(ctx, fieldsQuery, map[string]any{"includeArchived": true})
			if err != nil {
				return err
			}
			for _, held := range listed.Data.Fields {
				if held.Name == name && held.ArchivedAt != nil {
					return nil
				}
			}
			return fmt.Errorf("no archived definition named %q, answered %s", name, w.answered)
		})

	sc.Then(`^the definition is refused for a taken name$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedFor("CONFLICT")
	})

	sc.Then(`^the definition is refused for a (?:reserved|malformed) name$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.refusedFor("VALIDATION"); err != nil {
			return err
		}
		if strings.Contains(string(w.answered), "\"defineField\":{") {
			return fmt.Errorf("the graph answered a definition, answered %s", w.answered)
		}
		return nil
	})
}
