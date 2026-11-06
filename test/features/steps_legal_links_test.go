// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
)

// legalLinksQuery reads the addresses of the install's legal pages.
const legalLinksQuery = `{"query":"{ legalLinks { privacyUrl termsUrl } }"}`

// legalLinksAnswer is the envelope every legal links step reads.
type legalLinksAnswer struct {
	Data struct {
		LegalLinks *struct {
			PrivacyURL *string `json:"privacyUrl"`
			TermsURL   *string `json:"termsUrl"`
		} `json:"legalLinks"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// legalAddresses decodes the privacy and terms addresses the last read answered.
func (w *world) legalAddresses() (privacy, terms *string, err error) {
	var answer legalLinksAnswer
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return nil, nil, fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	if len(answer.Errors) > 0 || answer.Data.LegalLinks == nil {
		return nil, nil, fmt.Errorf("the graph answered no legal links, answered %s", w.answered)
	}
	return answer.Data.LegalLinks.PrivacyURL, answer.Data.LegalLinks.TermsURL, nil
}

// wantAddress reports whether an answered address reads as wanted.
func wantAddress(name string, got *string, want string) error {
	if got == nil {
		return fmt.Errorf("no %s address answered, want %q", name, want)
	}
	if *got != want {
		return fmt.Errorf("%s address = %q, want %q", name, *got, want)
	}
	return nil
}

// wantNoAddress reports whether an address stayed unanswered.
func wantNoAddress(name string, got *string) error {
	if got != nil {
		return fmt.Errorf("%s address = %q, want none", name, *got)
	}
	return nil
}

// registerLegalLinksSteps binds the legal links steps and the world lifecycle.
func registerLegalLinksSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding a user with an API token$`, func(ctx context.Context) error {
		if worldFrom(ctx).secret == "" {
			return fmt.Errorf("the scenario holds no token")
		}
		return nil
	})
	bindTenantSteps(sc)
	bindLegalLinksAsks(sc)
	bindLegalLinksAnswers(sc)
	sc.Then(`^the locale answered is "([^"]*)"$`, func(ctx context.Context, want string) error {
		return worldFrom(ctx).wantLocale(want)
	})
}

// bindLegalLinksAsks binds what the operator sets and what a caller asks.
func bindLegalLinksAsks(sc *godog.ScenarioContext) {
	sc.Given(`^the operator set the privacy address "([^"]*)"$`, func(ctx context.Context, address string) {
		worldFrom(ctx).graph.Legal.Privacy = address
	})
	sc.Given(`^the operator set the terms address "([^"]*)"$`, func(ctx context.Context, address string) {
		worldFrom(ctx).graph.Legal.Terms = address
	})
	sc.When(`^an anonymous caller reads the legal links$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphSpeaking(ctx, "", "", legalLinksQuery)
	})
	sc.When(`^the caller reads the legal links$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.postGraphSpeaking(ctx, w.secret, "", legalLinksQuery)
	})
	sc.When(`^an anonymous caller reads the locale and the legal links together$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphSpeaking(ctx, "", "",
			`{"query":"query AppLocale { locale legalLinks { privacyUrl termsUrl } }"}`)
	})
	sc.When(`^an anonymous caller reads the legal links beside the version$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphSpeaking(ctx, "", "", `{"query":"{ legalLinks { privacyUrl } version }"}`)
	})
	sc.When(`^an anonymous caller aliases the version as the legal links$`, func(ctx context.Context) error {
		return worldFrom(ctx).postGraphSpeaking(ctx, "", "", `{"query":"{ legalLinks: version }"}`)
	})
}

// bindLegalLinksAnswers binds what the graph must answer.
func bindLegalLinksAnswers(sc *godog.ScenarioContext) {
	sc.Then(`^the privacy address answered is "([^"]*)"$`, func(ctx context.Context, want string) error {
		privacy, _, err := worldFrom(ctx).legalAddresses()
		if err != nil {
			return err
		}
		return wantAddress("privacy", privacy, want)
	})
	sc.Then(`^the terms address answered is "([^"]*)"$`, func(ctx context.Context, want string) error {
		_, terms, err := worldFrom(ctx).legalAddresses()
		if err != nil {
			return err
		}
		return wantAddress("terms", terms, want)
	})
	sc.Then(`^no privacy address is answered$`, func(ctx context.Context) error {
		privacy, _, err := worldFrom(ctx).legalAddresses()
		if err != nil {
			return err
		}
		return wantNoAddress("privacy", privacy)
	})
	sc.Then(`^no terms address is answered$`, func(ctx context.Context) error {
		_, terms, err := worldFrom(ctx).legalAddresses()
		if err != nil {
			return err
		}
		return wantNoAddress("terms", terms)
	})
}
