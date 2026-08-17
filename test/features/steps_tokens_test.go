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
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
)

// scopeAnswer is the envelope every scope step reads.
type scopeAnswer struct {
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// postGraphScoped posts a graph request as the scenario's scoped token, keeping the status answered.
func (w *world) postGraphScoped(ctx context.Context, body string) error {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+w.scopedSecret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("posting the graph request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the graph answer: %w", err)
	}
	w.status = response.StatusCode
	w.answered = answered
	return nil
}

// scopeErrors returns the errors the last graph answer carried.
func (w *world) scopeErrors() (scopeAnswer, error) {
	var parsed scopeAnswer
	if err := json.Unmarshal(w.answered, &parsed); err != nil {
		return scopeAnswer{}, fmt.Errorf("reading the answer %s: %w", w.answered, err)
	}
	return parsed, nil
}

// answeredWithoutRefusal reports whether the last operation carried no error.
func (w *world) answeredWithoutRefusal() error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 0 {
		return fmt.Errorf("the operation was refused: %s", w.answered)
	}
	return nil
}

// refusedNaming reports whether the last operation was refused naming the given scope.
func (w *world) refusedNaming(scope string) error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 1 {
		return fmt.Errorf("errors = %v, want exactly one refusal", parsed.Errors)
	}
	if code := parsed.Errors[0].Extensions["code"]; code != "UNAUTHORIZED" {
		return fmt.Errorf("code = %v, want UNAUTHORIZED", code)
	}
	if named := parsed.Errors[0].Extensions["scope"]; named != scope {
		return fmt.Errorf("scope = %v, want %q", named, scope)
	}
	return nil
}

// registerTokenSteps binds the token scope steps and the world lifecycle.
func registerTokenSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.Given(`^the user holds a token scoped to "([^"]*)"$`, func(ctx context.Context, scopes string) error {
		w := worldFrom(ctx)
		secret, err := w.mintScopedSecretFor(ctx, w.ownerID, "scoped", apitoken.ParseScopes(scopes))
		if err != nil {
			return err
		}
		w.scopedSecret = secret
		return nil
	})

	sc.Given(`^the user holds a token that expired yesterday$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		secret, err := w.mintScopedSecretFor(ctx, w.ownerID, "spent", apitoken.Full())
		if err != nil {
			return err
		}
		w.scopedSecret = secret
		return w.expireSecret(ctx, secret)
	})

	sc.Given(`^a token minted before scopes existed$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		secret, err := w.mintScopedSecretFor(ctx, w.ownerID, "legacy", apitoken.Full())
		if err != nil {
			return err
		}
		w.scopedSecret = secret
		return nil
	})

	registerTokenOperations(sc)
	registerTokenOutcomes(sc)
	registerTokenSessionSteps(sc)
}

// registerTokenOperations binds the operations a scoped token attempts.
func registerTokenOperations(sc *godog.ScenarioContext) {
	sc.When(`^that token lists the contacts$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphScoped(ctx,
			`{"query":"{ contacts(first: 5) { edges { node { id name } } } }"}`)
	})

	sc.When(`^that token creates a contact named "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).postGraphScoped(ctx,
			fmt.Sprintf(`{"query":"mutation { createContact(name: \"%s\") { id } }"}`, name))
	})

	sc.When(`^that token creates a task titled "([^"]*)"$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).postGraphScoped(ctx, fmt.Sprintf(
			`{"query":"mutation { createTask(input: {title: \"%s\", dueOn: \"2026-08-01\"})`+
				` { task { id } } }"}`, title))
	})

	sc.When(`^that token disables a user$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphScoped(ctx, fmt.Sprintf(
			`{"query":"mutation { setUserDisabled(id: \"%s\", disabled: true) }"}`, uuid.Must(uuid.NewV7())))
	})

	sc.When(`^that token asks for the version$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphScoped(ctx, `{"query":"{ version }"}`)
	})
}

// registerTokenOutcomes binds the outcomes a scoped operation answers with.
func registerTokenOutcomes(sc *godog.ScenarioContext) {
	sc.Then(`^the list is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutRefusal()
	})

	sc.Then(`^the task is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutRefusal()
	})

	sc.Then(`^the contact is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutRefusal()
	})

	sc.Then(`^the operation is refused as unauthorized naming "([^"]*)"$`,
		func(ctx context.Context, scope string) error {
			return worldFrom(ctx).refusedNaming(scope)
		})

	sc.Then(`^the request is refused as an invalid token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.status != http.StatusUnauthorized {
			return fmt.Errorf("status = %d, want %d: %s", w.status, http.StatusUnauthorized, w.answered)
		}
		if !strings.Contains(string(w.answered), "invalid token") {
			return fmt.Errorf("answer = %s, want it to name an invalid token", w.answered)
		}
		return nil
	})
}
