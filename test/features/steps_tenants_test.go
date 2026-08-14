// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// tenantQuery reads the caller's own tenant.
const tenantQuery = `{"query":"{ tenant { id name } }"}`

// tenantAnswer is the envelope every tenant step reads.
type tenantAnswer struct {
	Data struct {
		Tenant struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"tenant"`
	} `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// postGraphWith posts a graph request carrying the given bearer secret, empty for none.
func (w *world) postGraphWith(ctx context.Context, secret, body string) ([]byte, error) {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("posting the graph request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the graph answer: %w", err)
	}
	w.answered = answered
	return answered, nil
}

// askTenant reads the tenant the given secret's caller stands in.
func (w *world) askTenant(ctx context.Context, secret string) error {
	_, err := w.postGraphWith(ctx, secret, tenantQuery)
	return err
}

// registerTenantSteps binds the tenant steps and the world lifecycle.
func registerTenantSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.Given(`^the tenant "([^"]*)" exists$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).seedTenant(ctx, name)
		return err
	})

	sc.Given(`^the caller is placed in the tenant "([^"]*)"$`,
		func(ctx context.Context, name string) error {
			w := worldFrom(ctx)
			return w.placeMember(ctx, w.ownerID, name)
		})

	sc.Given(`^a user "([^"]*)" holding a token is placed in the tenant "([^"]*)"$`,
		func(ctx context.Context, email, name string) error {
			w := worldFrom(ctx)
			userID, err := w.addUser(ctx, email, "Grace Hopper")
			if err != nil {
				return err
			}
			secret, err := w.mintSecretFor(ctx, userID, "tenant scenario")
			if err != nil {
				return err
			}
			w.altSecret = secret
			return w.placeMember(ctx, userID, name)
		})

	sc.When(`^the caller asks for its tenant$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.askTenant(ctx, w.secret)
	})

	sc.When(`^that token asks for its tenant$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.altSecret == "" {
			return fmt.Errorf("the scenario minted no second token")
		}
		return w.askTenant(ctx, w.altSecret)
	})

	sc.When(`^an anonymous caller asks for a tenant$`, func(ctx context.Context) error {
		return worldFrom(ctx).askTenant(ctx, "")
	})

	sc.Then(`^the store holds the tenant "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		var held int
		if err := w.pool.QueryRow(ctx,
			"SELECT count(*) FROM core.tenants WHERE name = $1", name).Scan(&held); err != nil {
			return fmt.Errorf("reading the tenants: %w", err)
		}
		if held != 1 {
			return fmt.Errorf("the store holds %d tenants named %q, want 1", held, name)
		}
		return nil
	})

	sc.Then(`^the tenant answered is "([^"]*)"$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		var answer tenantAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		if len(answer.Errors) > 0 {
			return fmt.Errorf("the graph refused the read, answered %s", w.answered)
		}
		if answer.Data.Tenant.Name != name {
			return fmt.Errorf("tenant = %q, want %q, answered %s", answer.Data.Tenant.Name, name, w.answered)
		}
		if answer.Data.Tenant.ID == "" {
			return fmt.Errorf("the tenant carries no id, answered %s", w.answered)
		}
		return nil
	})

	sc.Then(`^the ask is refused as unauthenticated$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		var answer tenantAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding %s: %w", w.answered, err)
		}
		if len(answer.Errors) == 0 {
			return fmt.Errorf("the graph accepted the anonymous ask, answered %s", w.answered)
		}
		if got := answer.Errors[0].Extensions["code"]; got != "UNAUTHENTICATED" {
			return fmt.Errorf("code = %v, want UNAUTHENTICATED, answered %s", got, w.answered)
		}
		return nil
	})
}
