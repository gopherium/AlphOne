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
)

// localeAnswer is the envelope every locale step reads.
type localeAnswer struct {
	Data struct {
		Locale    string `json:"locale"`
		SetLocale string `json:"setLocale"`
	} `json:"data"`
	Errors []struct {
		Message    string         `json:"message"`
		Extensions map[string]any `json:"extensions"`
	} `json:"errors"`
}

// postGraphSpeaking posts a graph request carrying the secret and the Accept-Language header.
func (w *world) postGraphSpeaking(ctx context.Context, secret, acceptLanguage, body string) error {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodPost, w.server.URL+"/api/graphql", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("building the graph request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	if acceptLanguage != "" {
		request.Header.Set("Accept-Language", acceptLanguage)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return fmt.Errorf("posting the graph request: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	answered, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("reading the graph answer: %w", err)
	}
	w.answered = answered
	return nil
}

// registerLocaleSteps binds the locale resolution steps and the world lifecycle.
func registerLocaleSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})

	sc.When(`^an anonymous caller asks for the locale$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphSpeaking(ctx, "", "", `{"query":"{ locale }"}`)
	})

	sc.When(`^an anonymous caller asks for the locale speaking "([^"]*)"$`,
		func(ctx context.Context, language string) error {
			return worldFrom(ctx).postGraphSpeaking(ctx, "", language, `{"query":"{ locale }"}`)
		})

	sc.When(`^the caller asks for the locale speaking "([^"]*)"$`,
		func(ctx context.Context, language string) error {
			w := worldFrom(ctx)
			return w.postGraphSpeaking(ctx, w.secret, language, `{"query":"{ locale }"}`)
		})

	sc.Given(`^the caller stored the locale "([^"]*)"$`, func(ctx context.Context, chosen string) error {
		w := worldFrom(ctx)
		body := fmt.Sprintf(`{"query":"mutation { setLocale(locale: \"%s\") }"}`, chosen)
		if err := w.postGraphSpeaking(ctx, w.secret, "", body); err != nil {
			return err
		}
		var answer localeAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding the store answer: %w", err)
		}
		if len(answer.Errors) != 0 {
			return fmt.Errorf("storing the locale was refused, answered %s", w.answered)
		}
		return nil
	})

	sc.When(`^the caller stores the locale "([^"]*)"$`, func(ctx context.Context, chosen string) error {
		w := worldFrom(ctx)
		body := fmt.Sprintf(`{"query":"mutation { setLocale(locale: \"%s\") }"}`, chosen)
		return w.postGraphSpeaking(ctx, w.secret, "", body)
	})

	sc.Then(`^the locale answered is "([^"]*)"$`, func(ctx context.Context, want string) error {
		w := worldFrom(ctx)
		var answer localeAnswer
		if err := json.Unmarshal(w.answered, &answer); err != nil {
			return fmt.Errorf("decoding the locale answer: %w", err)
		}
		if len(answer.Errors) != 0 {
			return fmt.Errorf("errors = %v, want the locale answered", answer.Errors)
		}
		if answer.Data.Locale != want {
			return fmt.Errorf("locale = %q, want %q", answer.Data.Locale, want)
		}
		return nil
	})

	sc.Then(`^the ask is refused naming the reason "([^"]*)"$`,
		func(ctx context.Context, want string) error {
			w := worldFrom(ctx)
			var answer localeAnswer
			if err := json.Unmarshal(w.answered, &answer); err != nil {
				return fmt.Errorf("decoding the refused answer: %w", err)
			}
			if len(answer.Errors) != 1 {
				return fmt.Errorf("errors = %v, want exactly one error", answer.Errors)
			}
			if got := answer.Errors[0].Extensions["reason"]; got != want {
				return fmt.Errorf("reason = %v, want %q, answered %s", got, want, w.answered)
			}
			return nil
		})
}
