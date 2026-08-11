// SPDX-License-Identifier: Elastic-2.0

package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gopherium/alphone/internal/mcp"
)

// toolNames lists every tool AlphOne advertises.
var toolNames = []string{"find_contacts", "get_contact", "list_my_tasks", "workload_summary"}

// stubGraph answers every operation with an empty data envelope.
func stubGraph() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
}

// connect returns a client session talking to a server over the given graph.
func connect(t *testing.T, graph http.Handler) *sdkmcp.ClientSession {
	t.Helper()
	server := mcp.NewServer(graph, "test")
	serverSide, clientSide := sdkmcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), serverSide, nil); err != nil {
		t.Fatalf("connecting the server: %v", err)
	}
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(), clientSide, nil)
	if err != nil {
		t.Fatalf("connecting the client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestServerAdvertisesEveryToolAsReadOnly(t *testing.T) {
	t.Parallel()

	session := connect(t, stubGraph())

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v, want nil", err)
	}
	var got []string
	for _, tool := range listed.Tools {
		got = append(got, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q is not marked read only", tool.Name)
		}
		if !tool.Annotations.IdempotentHint {
			t.Errorf("tool %q is not marked idempotent", tool.Name)
		}
		if tool.Description == "" {
			t.Errorf("tool %q carries no description", tool.Name)
		}
	}
	slices.Sort(got)
	if !slices.Equal(got, toolNames) {
		t.Errorf("tools = %v, want %v", got, toolNames)
	}
}

func TestHandlerServesTheSessionOverHTTP(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(mcp.Handler(stubGraph(), "test"))
	t.Cleanup(srv.Close)

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test", Version: "test"}, nil)
	session, err := client.Connect(t.Context(),
		&sdkmcp.StreamableClientTransport{Endpoint: srv.URL}, nil)

	if err != nil {
		t.Fatalf("Connect() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v, want nil", err)
	}
	if len(listed.Tools) != len(toolNames) {
		t.Errorf("tools = %d, want %d", len(listed.Tools), len(toolNames))
	}
}

func TestEveryToolReportsItIsNotImplementedYet(t *testing.T) {
	t.Parallel()

	session := connect(t, stubGraph())

	arguments := map[string]map[string]any{
		"get_contact": {"contact_id": "0198c000-0000-7000-8000-000000000001"},
	}
	for _, name := range toolNames {
		result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{
			Name:      name,
			Arguments: arguments[name],
		})
		if err != nil {
			t.Fatalf("CallTool(%q) error = %v, want a tool failure", name, err)
		}
		if !result.IsError {
			t.Errorf("CallTool(%q) succeeded, want the unimplemented refusal", name)
		}
	}
}

func TestToolCallsCarryTheCallerContext(t *testing.T) {
	t.Parallel()

	var seen string
	graph := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	session := connect(t, graph)

	if _, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "workload_summary"}); err != nil {
		t.Fatalf("CallTool() error = %v, want nil", err)
	}

	if seen != "" && seen != "application/json" {
		t.Errorf("the graph saw content type %q, want application/json", seen)
	}
}

func TestUnknownToolIsRefused(t *testing.T) {
	t.Parallel()

	session := connect(t, stubGraph())

	_, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "no_such_tool"})

	if err == nil {
		t.Fatal("CallTool() error = nil, want the unknown tool refused")
	}
	if !strings.Contains(err.Error(), "no_such_tool") {
		t.Errorf("error = %v, want it to name the tool", err)
	}
}
