// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// answering returns a graph handler replying with the given body.
func answering(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func TestRunDecodesTheAnsweredData(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{"version":"9.9.9"}}`)}
	var out struct {
		Version string `json:"version"`
	}

	err := run.execute(t.Context(), "{ version }", nil, &out)

	if err != nil {
		t.Fatalf("execute() error = %v, want nil", err)
	}
	if out.Version != "9.9.9" {
		t.Errorf("version = %q, want 9.9.9", out.Version)
	}
}

func TestRunSendsTheDocumentAndVariables(t *testing.T) {
	t.Parallel()

	var body string
	graph := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(read)
		body = string(read)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	run := &tools{graph: graph}

	if err := run.execute(t.Context(), "query($a: Int!){ x }", map[string]any{"a": 1}, &struct{}{}); err != nil {
		t.Fatalf("execute() error = %v, want nil", err)
	}

	if !strings.Contains(body, `"query":"query($a: Int!){ x }"`) {
		t.Errorf("body = %s, want it to carry the document", body)
	}
	if !strings.Contains(body, `"a":1`) {
		t.Errorf("body = %s, want it to carry the variables", body)
	}
}

func TestRunReportsTheFirstGraphError(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(
		`{"data":null,"errors":[{"message":"task: not found","extensions":{"code":"NOT_FOUND"}}]}`)}

	err := run.execute(t.Context(), "{ task }", nil, &struct{}{})

	if err == nil {
		t.Fatal("execute() error = nil, want the graph refusal")
	}
	if !strings.Contains(err.Error(), "task: not found") {
		t.Errorf("error = %v, want the message", err)
	}
	if !strings.Contains(err.Error(), "NOT_FOUND") {
		t.Errorf("error = %v, want the extensions code", err)
	}
}

func TestRunReportsAGraphErrorWithoutACode(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"errors":[{"message":"plain refusal"}]}`)}

	err := run.execute(t.Context(), "{ task }", nil, &struct{}{})

	if err == nil || err.Error() != "plain refusal" {
		t.Errorf("error = %v, want the bare message", err)
	}
}

func TestRunReportsANonOKStatus(t *testing.T) {
	t.Parallel()

	run := &tools{graph: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	})}

	err := run.execute(t.Context(), "{ version }", nil, &struct{}{})

	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %v, want the status reported", err)
	}
}

func TestRunReportsAnUnreadableAnswer(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`not json`)}

	err := run.execute(t.Context(), "{ version }", nil, &struct{}{})

	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Errorf("error = %v, want the decode failure", err)
	}
}

func TestRunCarriesTheCallerContext(t *testing.T) {
	t.Parallel()

	type marker struct{}
	var seen bool
	graph := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Context().Value(marker{}) == "carried"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	run := &tools{graph: graph}
	ctx := context.WithValue(t.Context(), marker{}, "carried")

	if err := run.execute(ctx, "{ version }", nil, &struct{}{}); err != nil {
		t.Fatalf("execute() error = %v, want nil", err)
	}

	if !seen {
		t.Error("the graph did not see the caller context")
	}
}

// errUnencodable reports a value that refuses to become JSON.
var errUnencodable = errors.New("mcp: unencodable")

// unmarshalable refuses to encode, forcing the request build to fail.
type unmarshalable struct{}

// MarshalJSON always fails.
func (unmarshalable) MarshalJSON() ([]byte, error) {
	return nil, errUnencodable
}

func TestRunReportsAnUnencodableOperation(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{"data":{}}`)}

	err := run.execute(t.Context(), "{ x }", map[string]any{"bad": unmarshalable{}}, &struct{}{})

	if err == nil || !strings.Contains(err.Error(), "encode") {
		t.Errorf("error = %v, want the encode failure", err)
	}
}

func TestExecutorPostsToTheGraphPath(t *testing.T) {
	t.Parallel()

	var path, method string
	graph := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{}}`))
	})

	if err := (&tools{graph: graph}).execute(t.Context(), "{ x }", nil, &struct{}{}); err != nil {
		t.Fatalf("execute() error = %v, want nil", err)
	}

	if path != "/api/graphql" || method != http.MethodPost {
		t.Errorf("request = %s %s, want POST /api/graphql", method, path)
	}
}

// recording returns a graph handler capturing the request body it received.
func recording(into *string, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(read)
		*into = string(read)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
}

func TestRunAcceptsAnAnswerWithoutData(t *testing.T) {
	t.Parallel()

	run := &tools{graph: answering(`{}`)}

	if err := run.execute(t.Context(), "{ x }", nil, &struct{}{}); err != nil {
		t.Errorf("execute() error = %v, want nil for an empty envelope", err)
	}
}
