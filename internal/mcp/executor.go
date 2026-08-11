// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
)

// graphPath is where every tool posts its operation.
const graphPath = "/api/graphql"

// graphError is one refusal the graph answered with.
type graphError struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

// Error names the refusal beside its code.
func (e graphError) Error() string {
	if code, ok := e.Extensions["code"].(string); ok && code != "" {
		return fmt.Sprintf("%s (%s)", e.Message, code)
	}
	return e.Message
}

// answer is the envelope the graph replies with.
type answer struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphError    `json:"errors"`
}

// execute posts one operation into the graph and decodes its data into out.
func (t *tools) execute(ctx context.Context, document string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": document, "variables": variables})
	if err != nil {
		return fmt.Errorf("mcp: encode operation: %w", err)
	}
	request := httptest.NewRequestWithContext(ctx, http.MethodPost, graphPath, bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	t.graph.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		return fmt.Errorf("mcp: the graph answered %d", recorder.Code)
	}
	var replied answer
	if err := json.Unmarshal(recorder.Body.Bytes(), &replied); err != nil {
		return fmt.Errorf("mcp: decode answer: %w", err)
	}
	if len(replied.Errors) > 0 {
		return replied.Errors[0]
	}
	if len(replied.Data) == 0 {
		return nil
	}
	return json.Unmarshal(replied.Data, out)
}
