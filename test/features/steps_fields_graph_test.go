// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"
)

// coreListingQuery is the fixed contact listing the byte identity scenario pins.
const coreListingQuery = `{"query":"{ contacts(first: 5) { edges { node { id name createdAt } } } }"}`

// postGraph posts a graph request with the world's bearer token.
func (w *world) postGraph(ctx context.Context, body string) ([]byte, error) {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+w.secret)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("posting the graph request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("reading the graph answer: %w", err)
	}
	return answered, nil
}

// registerFieldsGraphSteps binds the widened graph steps and the world lifecycle.
func registerFieldsGraphSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.Given(`^a contact named "([^"]*)"$`, func(ctx context.Context, name string) error {
		_, err := worldFrom(ctx).seedContact(ctx, name)
		return err
	})

	sc.Given(`^the core contact listing is captured$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		answered, err := w.postGraph(ctx, coreListingQuery)
		if err != nil {
			return err
		}
		w.captured = answered
		return nil
	})

	sc.Step(`^the field "([^"]*)" labelled "([^"]*)" of kind ([A-Z]+) is defined$`,
		func(ctx context.Context, name, label, kind string) error {
			return worldFrom(ctx).defineField(ctx, name, label, kind)
		})

	sc.When(`^the Contact type is introspected$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		const query = `{"query":"{ __type(name: \"Contact\") { fields { name type { name } } } }"}`
		raw, err := w.postGraph(ctx, query)
		if err != nil {
			return err
		}
		w.answered = raw
		return nil
	})

	sc.Then(`^the introspection lists "([^"]*)" answering the scalar "([^"]*)"$`,
		func(ctx context.Context, name, scalar string) error {
			var answer struct {
				Data struct {
					Type struct {
						Fields []struct {
							Name string `json:"name"`
							Type struct {
								Name string `json:"name"`
							} `json:"type"`
						} `json:"fields"`
					} `json:"__type"`
				} `json:"data"`
			}
			w := worldFrom(ctx)
			if err := json.Unmarshal(w.answered, &answer); err != nil {
				return fmt.Errorf("decoding %s: %w", w.answered, err)
			}
			for _, held := range answer.Data.Type.Fields {
				if held.Name != name {
					continue
				}
				if held.Type.Name != scalar {
					return fmt.Errorf("%s answers %q, want %q", name, held.Type.Name, scalar)
				}
				return nil
			}
			return fmt.Errorf("introspection lists no field %q, answered %s", name, w.answered)
		})

	sc.Then(`^the core contact listing answers byte identical to the capture$`,
		func(ctx context.Context) error {
			w := worldFrom(ctx)
			answered, err := w.postGraph(ctx, coreListingQuery)
			if err != nil {
				return err
			}
			if !bytes.Equal(answered, w.captured) {
				return fmt.Errorf("the listing changed, captured %s, answered %s", w.captured, answered)
			}
			return nil
		})
}
