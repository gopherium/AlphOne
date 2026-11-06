// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"reflect"
	"slices"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"

	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/graphres"
)

const (
	privacyAddress = "https://example.com/legal/privacy"
	termsAddress   = "https://example.com/legal/terms"
)

// legalLinksRead is the answer of a legal links read.
type legalLinksRead struct {
	Locale     string `json:"locale"`
	LegalLinks struct {
		PrivacyURL *string `json:"privacyUrl"`
		TermsURL   *string `json:"termsUrl"`
	} `json:"legalLinks"`
}

// anonymousOverLegalLinks returns a client with no identity over a resolver holding links.
func anonymousOverLegalLinks(t *testing.T, links graphres.LegalLinks) *gqlclient.Client {
	t.Helper()
	resolver := newAuthResolver(testkit.NewStore())
	resolver.Legal = links
	return newAnonymousGraphClient(t, resolver)
}

func TestAnAnonymousCallerReadsTheLegalLinks(t *testing.T) {
	t.Parallel()

	client := anonymousOverLegalLinks(t, graphres.LegalLinks{Privacy: privacyAddress, Terms: termsAddress})
	var answer legalLinksRead

	client.MustPost(`{ legalLinks { privacyUrl termsUrl } }`, &answer)

	links := answer.LegalLinks
	if links.PrivacyURL == nil || *links.PrivacyURL != privacyAddress {
		t.Errorf("privacyUrl = %v, want %q", links.PrivacyURL, privacyAddress)
	}
	if links.TermsURL == nil || *links.TermsURL != termsAddress {
		t.Errorf("termsUrl = %v, want %q", links.TermsURL, termsAddress)
	}
}

func TestALegalAddressLeftUnsetAnswersNone(t *testing.T) {
	t.Parallel()

	client := anonymousOverLegalLinks(t, graphres.LegalLinks{Terms: termsAddress})
	var answer legalLinksRead

	client.MustPost(`{ legalLinks { privacyUrl termsUrl } }`, &answer)

	if answer.LegalLinks.PrivacyURL != nil {
		t.Errorf("privacyUrl = %q, want none", *answer.LegalLinks.PrivacyURL)
	}
	if answer.LegalLinks.TermsURL == nil {
		t.Error("termsUrl = none, want the one address set")
	}
}

func TestTheBootReadTakesTheLocaleAndTheLinksInOneAsk(t *testing.T) {
	t.Parallel()

	client := anonymousOverLegalLinks(t, graphres.LegalLinks{Privacy: privacyAddress})
	var answer legalLinksRead

	client.MustPost(`query AppLocale { locale legalLinks { privacyUrl } }`, &answer)

	if answer.Locale == "" || answer.LegalLinks.PrivacyURL == nil {
		t.Errorf("answer = %+v, want the locale and the links together", answer)
	}
}

func TestAnAnonymousAskCannotReachBeyondTheLegalLinks(t *testing.T) {
	t.Parallel()

	client := anonymousOverLegalLinks(t, graphres.LegalLinks{Privacy: privacyAddress})

	for name, query := range map[string]string{
		"a private field beside them": `{ legalLinks { privacyUrl } version }`,
		"a private field aliased":     `{ legalLinks: version }`,
		"a root fragment":             `query { ...F } fragment F on Query { legalLinks { privacyUrl } }`,
		"a root inline fragment":      `{ ... on Query { legalLinks { privacyUrl } } }`,
		"the root type name":          `{ __typename legalLinks { privacyUrl } }`,
		"the schema":                  `{ __schema { types { name } } }`,
		"a type lookup":               `{ __type(name: "User") { name } }`,
	} {
		response, err := client.RawPost(query)
		if err != nil {
			t.Fatalf("%s RawPost() error = %v, want nil", name, err)
		}
		if got := firstErrorCode(t, response.Errors); got != "UNAUTHENTICATED" {
			t.Errorf("%s code = %q, want UNAUTHENTICATED", name, got)
		}
	}
}

func TestTheGateReadsTheOperationTheCallerChose(t *testing.T) {
	t.Parallel()

	client := anonymousOverLegalLinks(t, graphres.LegalLinks{Privacy: privacyAddress})
	document := `query Links { legalLinks { privacyUrl } } query Private { version }`

	refused, err := client.RawPost(document, gqlclient.Operation("Private"))
	if err != nil {
		t.Fatalf("RawPost(Private) error = %v, want nil", err)
	}
	admitted, err := client.RawPost(document, gqlclient.Operation("Links"))
	if err != nil {
		t.Fatalf("RawPost(Links) error = %v, want nil", err)
	}

	if got := firstErrorCode(t, refused.Errors); got != "UNAUTHENTICATED" {
		t.Errorf("Private code = %q, want UNAUTHENTICATED", got)
	}
	if len(admitted.Errors) != 0 {
		t.Errorf("Links errors = %s, want none", admitted.Errors)
	}
}

func TestLegalLinksHoldsOnlyPlainStrings(t *testing.T) {
	t.Parallel()

	links := reflect.TypeFor[model.LegalLinks]()
	names := make([]string, 0, links.NumField())
	for field := range links.Fields() {
		names = append(names, field.Name)
		if field.Type != reflect.TypeFor[*string]() {
			t.Errorf("LegalLinks.%s is %s, want an optional string", field.Name, field.Type)
		}
	}

	if !slices.Equal(names, []string{"PrivacyURL", "TermsURL"}) {
		t.Errorf("LegalLinks holds %v, want only the two addresses, since the anonymous gate reads root names alone", names)
	}
	if _, forced := reflect.TypeFor[graph.ResolverRoot]().MethodByName("LegalLinks"); forced {
		t.Error("LegalLinks has a resolver of its own, want a plain model an anonymous caller reads")
	}
}
