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

// anonymousFieldQuery reads one field of any contact, naming no contact the caller could see.
const anonymousFieldQuery = `{"query":"{ contacts(first: 1) { edges { node { %s } } } }"}`

// secretOf returns the token of the user placed in the named tenant.
func (w *world) secretOf(tenant string) (string, error) {
	secret, placed := w.members[tenant]
	if !placed {
		return "", fmt.Errorf("the scenario placed no user in the tenant %q", tenant)
	}
	return secret, nil
}

// registerFieldsTenantsSteps binds the steps of fields kept per tenant and the world lifecycle.
func registerFieldsTenantsSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newImportWorld(t)), nil
	})
	bindFieldsCatalogSteps(sc)
	bindFieldValueSteps(sc)
	bindIntrospectionSteps(sc)
	bindTenantSteps(sc)
	bindImportSteps(sc)

	sc.Given(`^the tenant "([^"]*)" defines the field "([^"]*)" labelled "([^"]*)" of kind ([A-Z]+)$`,
		func(ctx context.Context, tenant, name, label, kind string) error {
			w := worldFrom(ctx)
			secret, err := w.secretOf(tenant)
			if err != nil {
				return err
			}
			answer, err := w.operationAs(ctx, secret, defineFieldMutation,
				map[string]any{"name": name, "label": label, "kind": kind})
			if err != nil {
				return err
			}
			if len(answer.Errors) > 0 {
				return fmt.Errorf("the tenant %q could not define %q, answered %s", tenant, name, w.answered)
			}
			return nil
		})

	sc.When(`^the tenant "([^"]*)" queries the contact for the field "([^"]*)"$`,
		func(ctx context.Context, tenant, name string) error {
			w := worldFrom(ctx)
			secret, err := w.secretOf(tenant)
			if err != nil {
				return err
			}
			body, err := json.Marshal(map[string]any{
				"query":     fmt.Sprintf(contactFieldQuery, name),
				"variables": map[string]any{"id": w.lastContact.String()},
			})
			if err != nil {
				return fmt.Errorf("encoding the read: %w", err)
			}
			_, err = w.postGraphWith(ctx, secret, string(body))
			return err
		})

	sc.When(`^the tenant "([^"]*)" introspects the Contact type$`, func(ctx context.Context, tenant string) error {
		w := worldFrom(ctx)
		secret, err := w.secretOf(tenant)
		if err != nil {
			return err
		}
		_, err = w.postGraphWith(ctx, secret, contactTypeQuery)
		return err
	})

	sc.When(`^an anonymous caller asks a contact for the field "([^"]*)"$`,
		func(ctx context.Context, name string) error {
			_, err := worldFrom(ctx).postGraphWith(ctx, "", fmt.Sprintf(anonymousFieldQuery, name))
			return err
		})

	sc.Then(`^the write is accepted$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var answer struct {
			Data struct {
				WriteContactFields bool `json:"writeContactFields"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		if len(answer.Errors) > 0 || !answer.Data.WriteContactFields {
			return fmt.Errorf("the graph did not accept the write, answered %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the introspection does not list "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		scalars, err := w.introspectedScalars()
		if err != nil {
			return err
		}
		if len(scalars) == 0 {
			return fmt.Errorf("the introspection listed no field at all, answered %s", w.answered)
		}
		if _, listed := scalars[name]; listed {
			return fmt.Errorf("the introspection lists %q, answered %s", name, w.answered)
		}
		return nil
	})

	sc.Then(`^the answer does not name "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		if !strings.Contains(string(w.answered), `"errors"`) {
			return fmt.Errorf("the graph answered the anonymous read, answered %s", w.answered)
		}
		if strings.Contains(string(w.answered), name) {
			return fmt.Errorf("the answer names %q, answered %s", name, w.answered)
		}
		return nil
	})
}
