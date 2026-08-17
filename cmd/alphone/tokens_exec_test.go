// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// sessionOf logs the seeded admin in against the running binary and returns its session cookie.
func sessionOf(t *testing.T, addr string) *http.Cookie {
	t.Helper()
	document := `{"query":"mutation { login(email: \"admin@example.com\",` +
		` password: \"correct horse battery\") { me { email } } }"}`
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, "http://"+addr+"/api/graphql", strings.NewReader(document))
	if err != nil {
		t.Fatalf("building the login request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the login request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, _ := io.ReadAll(response.Body)
	for _, cookie := range response.Cookies() {
		if strings.Contains(cookie.Name, "session") {
			return cookie
		}
	}
	t.Fatalf("the login answered no session cookie: %s", answered)
	return nil
}

// listedTokens is the answer the token listing carries.
type listedTokens struct {
	Data struct {
		APITokens []struct {
			ID     string   `json:"id"`
			Name   string   `json:"name"`
			Scopes []string `json:"scopes"`
		} `json:"apiTokens"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// postGraphAsSession posts a graph request carrying the given session cookie.
func postGraphAsSession(t *testing.T, addr string, session *http.Cookie, body string) listedTokens {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodPost, "http://"+addr+"/api/graphql", strings.NewReader(body))
	if err != nil {
		t.Fatalf("building the graph request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(session)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the graph request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("reading the graph answer: %v", err)
	}
	var envelope listedTokens
	if err := json.Unmarshal(answered, &envelope); err != nil {
		t.Fatalf("decoding %s: %v", answered, err)
	}
	return envelope
}

func TestMainBinaryServesTheTokenListing(t *testing.T) {
	t.Parallel()

	addr, _ := servedBinary(t, testDatabaseURL(t))
	session := sessionOf(t, addr)

	listed := postGraphAsSession(t, addr, session, `{"query":"{ apiTokens { id name scopes } }"}`)

	if len(listed.Errors) != 0 {
		t.Fatalf("errors = %v, want none, the resolver must reach a token store", listed.Errors)
	}
	if len(listed.Data.APITokens) != 1 {
		t.Fatalf("listed %d tokens, want the one the harness minted", len(listed.Data.APITokens))
	}
	if got := listed.Data.APITokens[0].Name; got != "exec" {
		t.Errorf("name = %q, want exec", got)
	}
}
