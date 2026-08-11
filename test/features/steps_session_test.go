// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gopherium/alphone/internal/postgres"
)

// bearerTransport adds the API token to every MCP request.
type bearerTransport struct {
	secret string
	base   http.RoundTripper
}

// RoundTrip sends the request carrying the bearer credential.
func (t bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	cloned := r.Clone(r.Context())
	if t.secret != "" {
		cloned.Header.Set("Authorization", "Bearer "+t.secret)
	}
	return t.base.RoundTrip(cloned)
}

// connect opens an MCP session carrying the given credential.
func (w *world) connect(ctx context.Context, secret string) error {
	client := mcp.NewClient(&mcp.Implementation{Name: "scenario", Version: "test"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   w.server.URL + "/api/mcp",
		HTTPClient: &http.Client{Transport: bearerTransport{secret: secret, base: http.DefaultTransport}},
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		w.connErr = err
		return nil
	}
	w.session = session
	w.t.Cleanup(func() { _ = session.Close() })
	return nil
}

// registerSessionSteps binds the MCP session steps and the world lifecycle.
func registerSessionSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.Given(`^the token is revoked$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return postgres.NewTokenStore(w.pool).Revoke(ctx, w.ownerID, w.tokenID)
	})

	sc.When(`^the agent connects with the token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.connect(ctx, w.secret)
	})

	sc.When(`^the agent connects without a token$`, func(ctx context.Context) error {
		return worldFrom(ctx).connect(ctx, "")
	})

	sc.Then(`^the session advertises exactly these tools$`, func(ctx context.Context, table *godog.Table) error {
		w := worldFrom(ctx)
		if w.session == nil {
			return fmt.Errorf("the agent never connected: %v", w.connErr)
		}
		listed, err := w.session.ListTools(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing tools: %w", err)
		}
		var got []string
		for _, tool := range listed.Tools {
			got = append(got, tool.Name)
		}
		var want []string
		for _, row := range table.Rows {
			want = append(want, strings.TrimSpace(row.Cells[0].Value))
		}
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			return fmt.Errorf("tools = %v, want %v", got, want)
		}
		return nil
	})

	sc.Then(`^every advertised tool is marked read only$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		listed, err := w.session.ListTools(ctx, nil)
		if err != nil {
			return fmt.Errorf("listing tools: %w", err)
		}
		for _, tool := range listed.Tools {
			if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
				return fmt.Errorf("tool %q is not marked read only", tool.Name)
			}
		}
		return nil
	})

	sc.Then(`^the connection is refused as unauthorized$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.connErr == nil {
			return fmt.Errorf("the connection succeeded, want it refused")
		}
		refusal := w.connErr.Error()
		if !strings.Contains(refusal, "401") && !strings.Contains(refusal, "Unauthorized") {
			return fmt.Errorf("error = %v, want an unauthorized refusal", w.connErr)
		}
		return nil
	})
}
