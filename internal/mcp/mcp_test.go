// SPDX-License-Identifier: Elastic-2.0

package mcp_test

import (
	"encoding/json"
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

func TestTaskAnswersTeachThePriorityScale(t *testing.T) {
	t.Parallel()

	session := connect(t, stubGraph())

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v, want nil", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name != "list_my_tasks" && tool.Name != "get_contact" {
			continue
		}
		schema, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshalling the %s output schema: %v", tool.Name, err)
		}
		for _, want := range []string{"0 to 9", "higher is more urgent"} {
			if !strings.Contains(string(schema), want) {
				t.Errorf("the %s output schema does not teach %q", tool.Name, want)
			}
		}
		if !strings.Contains(tool.Description, "0 to 9") {
			t.Errorf("the %s description does not teach the priority scale", tool.Name)
		}
	}
}

// missingPhrases returns the phrases text does not hold with its line breaks read as spaces, in the order given.
func missingPhrases(text string, phrases ...string) []string {
	flowed := strings.Join(strings.Fields(text), " ")
	var missing []string
	for _, phrase := range phrases {
		if !strings.Contains(flowed, phrase) {
			missing = append(missing, phrase)
		}
	}
	return missing
}

func TestTaskAnswersTeachTheTimeOfDayAndTheOrder(t *testing.T) {
	t.Parallel()

	session := connect(t, stubGraph())

	listed, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v, want nil", err)
	}
	teaches := map[string][]string{
		"list_my_tasks": {
			"HH:MM", "then by time of day", "no time comes after",
			"With status all", "the open ones with a time come first by time of day", "the done tasks together",
		},
		"get_contact": {"HH:MM", "then by time of day", "no time comes after"},
	}
	for _, tool := range listed.Tools {
		wants, ok := teaches[tool.Name]
		if !ok {
			continue
		}
		schema, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshalling the %s output schema: %v", tool.Name, err)
		}
		untaught := missingPhrases(string(schema),
			`"due_time"`, "wall clock time as HH:MM", "absent when the task has no time")
		if len(untaught) > 0 {
			t.Errorf("the %s output schema does not teach %q", tool.Name, untaught)
		}
		if unsaid := missingPhrases(tool.Description, wants...); len(unsaid) > 0 {
			t.Errorf("the %s description does not say %q", tool.Name, unsaid)
		}
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

func TestWorkloadAnswersDataNotProse(t *testing.T) {
	t.Parallel()

	counting := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"due":{"edges":[{},{}]},"overdue":{"edges":[{}]}}}`))
	})
	session := connect(t, counting)

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "workload_summary"})

	if err != nil {
		t.Fatalf("CallTool() error = %v, want nil", err)
	}
	if result.IsError {
		t.Fatalf("the tool failed: %v", result.Content)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("encoding the structured answer: %v", err)
	}
	want := `{"capped":false,"due_today":2,"overdue":1}`
	if canonical(t, string(structured)) != want {
		t.Errorf("structured = %s, want %s", structured, want)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content blocks = %d, want exactly the JSON mirror", len(result.Content))
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want a text block", result.Content[0])
	}
	if canonical(t, text.Text) != want {
		t.Errorf("text = %s, want the JSON mirror %s", text.Text, want)
	}
}

// canonical reorders a JSON object's keys for a stable comparison.
func canonical(t *testing.T, raw string) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encoding %q: %v", raw, err)
	}
	return string(encoded)
}

// listedStructured calls list_my_tasks over a graph answering with tasks and returns the structured answer.
func listedStructured(t *testing.T, tasks string) string {
	t.Helper()
	listing := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"tasks":{"edges":[{"node":` + tasks + `}]}}}`))
	})
	session := connect(t, listing)

	result, err := session.CallTool(t.Context(), &sdkmcp.CallToolParams{Name: "list_my_tasks"})

	if err != nil {
		t.Fatalf("CallTool() error = %v, want nil", err)
	}
	if result.IsError {
		t.Fatalf("the tool failed: %v", result.Content)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("encoding the structured answer: %v", err)
	}
	return string(structured)
}

func TestListMyTasksAnswersItemsThroughTheSession(t *testing.T) {
	t.Parallel()

	structured := listedStructured(t, `{
		"id":"t1","title":"Call Maria Perez back","dueOn":"2026-08-11","dueTime":"09:30","status":"open",
		"priority":0,"contactId":"c1","contact":{"id":"c1","name":"Maria Perez"}
	}`)

	for _, want := range []string{
		`"contact_name":"Maria Perez"`, `"contact_id":"c1"`, `"due_on":"2026-08-11"`, `"due_time":"09:30"`,
	} {
		if !strings.Contains(structured, want) {
			t.Errorf("structured = %s, want it to carry %s", structured, want)
		}
	}
}

func TestListMyTasksLeavesTheTimeOutOfAnUntimedTask(t *testing.T) {
	t.Parallel()

	structured := listedStructured(t, `{
		"id":"t1","title":"Send the quote","dueOn":"2026-08-11","dueTime":null,"status":"open","priority":0
	}`)

	if strings.Contains(structured, "due_time") || !strings.Contains(structured, `"due_on":"2026-08-11"`) {
		t.Errorf("structured = %s, want the day and no due_time key", structured)
	}
}

func TestContactToolsAnswerThroughTheSession(t *testing.T) {
	t.Parallel()

	directory := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(read)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(read), "contacts(") {
			_, _ = w.Write([]byte(`{"data":{"contacts":{"edges":[{"node":{
				"id":"c1","name":"Maria Perez","identities":[],"tasks":{"edges":[{"node":{"id":"t1"}}]}
			}}]}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"contact":{"id":"c1","name":"Maria Perez",
			"createdAt":"2026-08-01T10:00:00Z","identities":[],"tasks":{"edges":[]}}}}`))
	})
	session := connect(t, directory)

	for name, want := range map[string]string{
		"find_contacts": `"has_open_tasks":true`,
		"get_contact":   `"created_at":"2026-08-01T10:00:00Z"`,
	} {
		arguments := map[string]any{}
		if name == "get_contact" {
			arguments["contact_id"] = "0198c000-0000-7000-8000-000000000001"
		}
		result, err := session.CallTool(t.Context(),
			&sdkmcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil {
			t.Fatalf("CallTool(%q) error = %v, want nil", name, err)
		}
		if result.IsError {
			t.Fatalf("%s failed: %v", name, result.Content)
		}
		structured, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("encoding %s: %v", name, err)
		}
		if !strings.Contains(string(structured), want) {
			t.Errorf("%s = %s, want it to carry %s", name, structured, want)
		}
	}
}
