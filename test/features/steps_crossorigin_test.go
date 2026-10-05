// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/gopherium/alphone/internal/server"
)

// otherSitePage is the origin of a page on another site.
const otherSitePage = "https://pages.example.net"

// siblingSitePage is the origin of a page on a sibling subdomain.
const siblingSitePage = "https://shop.example.com"

// relayedHost is the host a proxy that renames the host sends in place of the one the visitor typed.
const relayedHost = "alphone:8080"

// agentHandshake is the first message an agent posts to the MCP endpoint.
const agentHandshake = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18",` +
	`"capabilities":{},"clientInfo":{"name":"n8n","version":"1.0.0"}}}`

// browserPage is the page a browser posts a form from and the fetch site it names, empty for an older browser.
type browserPage struct {
	origin    string
	fetchSite string
}

// pageOn returns the page a browser posts from on the named site.
func (w *world) pageOn(site string) browserPage {
	switch site {
	case "another site":
		return browserPage{origin: otherSitePage, fetchSite: "cross-site"}
	case "a sibling site":
		return browserPage{origin: siblingSitePage, fetchSite: "same-site"}
	}
	return browserPage{origin: w.server.URL, fetchSite: "same-origin"}
}

// olderPageOn returns the page an older browser, sending Origin alone, posts from on the named site.
func (w *world) olderPageOn(site string) browserPage {
	return browserPage{origin: w.pageOn(site).origin}
}

// formPost returns operations as a multipart graph form from page, carrying cookie when one is given.
func (w *world) formPost(ctx context.Context, page browserPage, cookie, operations string) (*http.Request, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("operations", operations); err != nil {
		return nil, fmt.Errorf("writing the operations field: %w", err)
	}
	if err := form.WriteField("map", "{}"); err != nil {
		return nil, fmt.Errorf("writing the map field: %w", err)
	}
	if err := form.Close(); err != nil {
		return nil, fmt.Errorf("closing the form: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, w.server.URL+"/api/graphql", &body)
	if err != nil {
		return nil, fmt.Errorf("building the form post: %w", err)
	}
	request.Header.Set("Content-Type", form.FormDataContentType())
	request.Header.Set("Origin", page.origin)
	if page.fetchSite != "" {
		request.Header.Set("Sec-Fetch-Site", page.fetchSite)
	}
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	return request, nil
}

// postForm posts operations as a multipart graph form from page, carrying cookie when one is given.
func (w *world) postForm(ctx context.Context, page browserPage, cookie, operations string) error {
	request, err := w.formPost(ctx, page, cookie, operations)
	if err != nil {
		return err
	}
	return w.send(request)
}

// postRelayedForm posts operations from page as a proxy relays it, renaming the host and forwarding the original.
func (w *world) postRelayedForm(ctx context.Context, page browserPage, cookie, operations string) error {
	request, err := w.formPost(ctx, page, cookie, operations)
	if err != nil {
		return err
	}
	request.Header.Set("X-Forwarded-Host", request.URL.Host)
	request.Header.Set("X-Forwarded-Proto", request.URL.Scheme)
	request.Host = relayedHost
	return w.send(request)
}

// send sends request and keeps the status, body and cookies answered.
func (w *world) send(request *http.Request) error {
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("sending %s %s: %w", request.Method, request.URL.Path, err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the answer: %w", err)
	}
	w.status = response.StatusCode
	w.answered = answered
	w.cookies = response.Cookies()
	return nil
}

// createContactOperations returns the graph operations creating a contact named name.
func createContactOperations(name string) string {
	return fmt.Sprintf(`{"query":"mutation { createContact(name: \"%s\") { id } }"}`, name)
}

// contactNames is the answer the session's contact listing carries.
type contactNames struct {
	Data struct {
		Contacts struct {
			Edges []struct {
				Node struct {
					Name string `json:"name"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"contacts"`
	} `json:"data"`
}

// sessionHolds reports whether the user's session lists a contact named name.
func (w *world) sessionHolds(ctx context.Context, name string) (bool, error) {
	if err := w.postGraphAsSession(ctx, `{"query":"{ contacts(first: 20) { edges { node { name } } } }"}`); err != nil {
		return false, err
	}
	var listed contactNames
	if err := json.Unmarshal(w.answered, &listed); err != nil {
		return false, fmt.Errorf("reading the contact listing %s: %w", w.answered, err)
	}
	for _, edge := range listed.Data.Contacts.Edges {
		if edge.Node.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// registerCrossOriginSteps binds the token steps and the steps posting from browser pages.
func registerCrossOriginSteps(sc *godog.ScenarioContext, t *testing.T) {
	registerTokenSteps(sc, t)

	sc.Given(`^the user is signed in through the browser$`, func(ctx context.Context) error {
		return worldFrom(ctx).signIn(ctx)
	})

	sc.When(`^a page on (another site|a sibling site|this site) posts a form creating the contact "([^"]*)"`+
		` through the user's browser$`, func(ctx context.Context, site, name string) error {
		w := worldFrom(ctx)
		return w.postForm(ctx, w.pageOn(site), w.sessionValue, createContactOperations(name))
	})

	sc.When(`^an older browser posts a form creating the contact "([^"]*)" from a page on (another site|this site)$`,
		func(ctx context.Context, name, site string) error {
			w := worldFrom(ctx)
			return w.postForm(ctx, w.olderPageOn(site), w.sessionValue, createContactOperations(name))
		})

	sc.When(`^an older browser posts a form creating the contact "([^"]*)" from a page on this site`+
		` through a proxy that renames the host$`, func(ctx context.Context, name string) error {
		w := worldFrom(ctx)
		return w.postRelayedForm(ctx, w.olderPageOn("this site"), w.sessionValue, createContactOperations(name))
	})

	sc.When(`^a page on another site signs the user in through the visitor's browser$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.postForm(ctx, w.pageOn("another site"), "", fmt.Sprintf(
			`{"query":"mutation { login(email: \"%s\", password: \"%s\") { me { email } } }"}`,
			ownerEmail, ownerPassword))
	})

	sc.When(`^a page on another site signs the user out through the user's browser$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.postForm(ctx, w.pageOn("another site"), w.sessionValue, `{"query":"mutation { logout }"}`)
	})

	sc.When(`^an agent posts an MCP handshake with the token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		request, err := http.NewRequestWithContext(
			ctx, http.MethodPost, w.server.URL+"/api/mcp", strings.NewReader(agentHandshake))
		if err != nil {
			return fmt.Errorf("building the handshake: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Authorization", "Bearer "+w.secret)
		return w.send(request)
	})

	registerRefusalOutcomes(sc)
	registerAnswerOutcomes(sc)
}

// registerRefusalOutcomes binds the outcomes a refused browser post answers with.
func registerRefusalOutcomes(sc *godog.ScenarioContext) {
	sc.Then(`^the request is refused with the code "([^"]*)"$`, func(ctx context.Context, code string) error {
		w := worldFrom(ctx)
		if w.status != http.StatusForbidden {
			return fmt.Errorf("status = %d, want %d: %s", w.status, http.StatusForbidden, w.answered)
		}
		var refused struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(w.answered, &refused); err != nil {
			return fmt.Errorf("reading the refusal %s: %w", w.answered, err)
		}
		if refused.Code != code {
			return fmt.Errorf("code = %q, want %q", refused.Code, code)
		}
		return nil
	})

	sc.Then(`^no session cookie is answered$`, func(ctx context.Context) error {
		for _, cookie := range worldFrom(ctx).cookies {
			if cookie.Name == server.SessionCookieName {
				return fmt.Errorf("the answer set the session cookie %q", cookie.Value)
			}
		}
		return nil
	})
}

// registerAnswerOutcomes binds the outcomes an answered browser post or agent handshake leaves.
func registerAnswerOutcomes(sc *godog.ScenarioContext) {
	sc.Then(`^the form is answered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.status != http.StatusOK {
			return fmt.Errorf("status = %d, want %d: %s", w.status, http.StatusOK, w.answered)
		}
		return w.answeredWithoutError()
	})

	sc.Then(`^the handshake is answered$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if w.status != http.StatusOK {
			return fmt.Errorf("status = %d, want %d: %s", w.status, http.StatusOK, w.answered)
		}
		return nil
	})

	sc.Then(`^the user's session still answers$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.postGraphAsSession(ctx, `{"query":"{ me { email } }"}`); err != nil {
			return err
		}
		if !strings.Contains(string(w.answered), ownerEmail) {
			return fmt.Errorf("me = %s, want the session still signed in as %s", w.answered, ownerEmail)
		}
		return nil
	})

	sc.Then(`^the user's session finds (no|the) contact named "([^"]*)"$`,
		func(ctx context.Context, which, name string) error {
			held, err := worldFrom(ctx).sessionHolds(ctx, name)
			if err != nil {
				return err
			}
			if held != (which == "the") {
				return fmt.Errorf("the session lists %q: %t, want %t", name, held, which == "the")
			}
			return nil
		})
}
