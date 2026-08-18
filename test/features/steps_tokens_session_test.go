// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/gopherium/alphone/internal/server"
)

// mintedAnswer is the answer the mint mutation carries.
type mintedAnswer struct {
	Data struct {
		APITokenCreate struct {
			Secret string `json:"secret"`
		} `json:"apiTokenCreate"`
	} `json:"data"`
}

// tokenListing is the answer the session's token listing carries.
type tokenListing struct {
	Data struct {
		APITokens []struct {
			Name      string   `json:"name"`
			Scopes    []string `json:"scopes"`
			ExpiresAt *string  `json:"expiresAt"`
		} `json:"apiTokens"`
	} `json:"data"`
}

// signIn logs the scenario's owner in and remembers the session cookie answered.
func (w *world) signIn(ctx context.Context) error {
	return w.signInAs(ctx, ownerEmail, &w.sessionValue)
}

// signInAs logs the named user in and keeps the session cookie answered.
func (w *world) signInAs(ctx context.Context, email string, keep *string) error {
	document, err := json.Marshal(map[string]string{"query": fmt.Sprintf(
		`mutation { login(email: %q, password: %q) { me { email } } }`, email, ownerPassword)})
	if err != nil {
		return fmt.Errorf("encoding the login request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(string(document)))
	if err != nil {
		return fmt.Errorf("building the login request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("posting the login request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, _ := io.ReadAll(response.Body)
	for _, cookie := range response.Cookies() {
		if cookie.Name == server.SessionCookieName {
			*keep = cookie.Name + "=" + cookie.Value
			return nil
		}
	}
	return fmt.Errorf("the login answered no session cookie: %s", answered)
}

// postGraphAsMember posts a graph request carrying the member's own session.
func (w *world) postGraphAsMember(ctx context.Context, body string) error {
	if w.memberValue == "" {
		if err := w.signInAs(ctx, memberEmail, &w.memberValue); err != nil {
			return err
		}
	}
	return w.postGraphWithCookie(ctx, w.memberValue, body)
}

// postGraphAsSession posts a graph request carrying the remembered session cookie.
func (w *world) postGraphAsSession(ctx context.Context, body string) error {
	if w.sessionValue == "" {
		if err := w.signIn(ctx); err != nil {
			return err
		}
	}
	return w.postGraphWithCookie(ctx, w.sessionValue, body)
}

// postGraphWithCookie posts a graph request carrying the given session cookie.
func (w *world) postGraphWithCookie(ctx context.Context, cookie, body string) error {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", cookie)
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

// registerTokenSessionSteps binds the steps a login session drives.
func registerTokenSessionSteps(sc *godog.ScenarioContext) {
	sc.When(`^that token lists the api tokens$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphScoped(ctx, `{"query":"{ apiTokens { id name } }"}`)
	})

	sc.Then(`^the operation is refused as unauthorized for token management$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedNaming("tokens:read")
	})

	sc.When(`^the user's session revokes that token$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.postGraphAsSession(ctx, fmt.Sprintf(
			`{"query":"mutation { apiTokenRevoke(id: \"%s\") }"}`, w.scopedID))
	})

	sc.When(`^the user's session mints a token named "([^"]*)" scoped to "([^"]*)" lasting (\d+) days$`,
		func(ctx context.Context, name, scopes string, days int) error {
			return worldFrom(ctx).postGraphAsSession(ctx, fmt.Sprintf(
				`{"query":"mutation { apiTokenCreate(name: \"%s\", scopes: [\"%s\"], ttlDays: %d)`+
					` { secret token { name } } }"}`, name, scopes, days))
		})

	sc.Then(`^the answer carries a secret beginning with "([^"]*)"$`, func(ctx context.Context, prefix string) error {
		var minted mintedAnswer
		if err := json.Unmarshal(worldFrom(ctx).answered, &minted); err != nil {
			return fmt.Errorf("reading the mint answer: %w", err)
		}
		if !strings.HasPrefix(minted.Data.APITokenCreate.Secret, prefix) {
			return fmt.Errorf("secret = %q, want the %q prefix", minted.Data.APITokenCreate.Secret, prefix)
		}
		return nil
	})

	registerTokenListingSteps(sc)
}

// registerTokenListingSteps binds the steps reading the session's token listing.
func registerTokenListingSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the session's token list shows "([^"]*)" scoped to "([^"]*)" expiring in (\d+) days$`,
		func(ctx context.Context, name, scopes string, days int) error {
			w := worldFrom(ctx)
			if err := w.postGraphAsSession(ctx, `{"query":"{ apiTokens { name scopes expiresAt } }"}`); err != nil {
				return err
			}
			if err := w.listingShows(name, scopes, true); err != nil {
				return err
			}
			return w.listingExpiresIn(name, days)
		})

	sc.Then(`^the list never shows a secret$`, func(ctx context.Context) error {
		if strings.Contains(string(worldFrom(ctx).answered), "secret") {
			return fmt.Errorf("the listing carries a secret: %s", worldFrom(ctx).answered)
		}
		return nil
	})

	sc.Then(`^the session's token list shows it scoped to "([^"]*)" never expiring$`,
		func(ctx context.Context, scopes string) error {
			w := worldFrom(ctx)
			if err := w.postGraphAsSession(ctx, `{"query":"{ apiTokens { name scopes expiresAt } }"}`); err != nil {
				return err
			}
			return w.listingShows("legacy", scopes, false)
		})
}

// listingExpiresIn reports whether the named token in the last listing ends the given days from now.
func (w *world) listingExpiresIn(name string, days int) error {
	var listed tokenListing
	if err := json.Unmarshal(w.answered, &listed); err != nil {
		return fmt.Errorf("reading the listing: %w", err)
	}
	for _, token := range listed.Data.APITokens {
		if token.Name != name || token.ExpiresAt == nil {
			continue
		}
		ends, err := time.Parse(time.RFC3339, *token.ExpiresAt)
		if err != nil {
			return fmt.Errorf("reading the expiry %q: %w", *token.ExpiresAt, err)
		}
		asked := time.Duration(days) * 24 * time.Hour
		if drift := time.Until(ends) - asked; drift > time.Hour || drift < -time.Hour {
			return fmt.Errorf("expires in %v, want about %v", time.Until(ends), asked)
		}
		return nil
	}
	return fmt.Errorf("the listing holds no expiring token named %q: %s", name, w.answered)
}

// listingShows reports whether the last listing carries one token with the given scopes and expiry.
func (w *world) listingShows(name, scopes string, expiring bool) error {
	var listed tokenListing
	if err := json.Unmarshal(w.answered, &listed); err != nil {
		return fmt.Errorf("reading the listing: %w", err)
	}
	for _, token := range listed.Data.APITokens {
		if token.Name != name {
			continue
		}
		if got := strings.Join(token.Scopes, " "); got != scopes {
			return fmt.Errorf("scopes = %q, want %q", got, scopes)
		}
		if expiring != (token.ExpiresAt != nil) {
			return fmt.Errorf("expiresAt = %v, want expiring %v", token.ExpiresAt, expiring)
		}
		return nil
	}
	return fmt.Errorf("the listing holds no token named %q: %s", name, w.answered)
}
