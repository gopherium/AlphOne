// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/graphres"
)

func TestTheLegalAddressesAreOptional(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.legal != (graphres.LegalLinks{}) {
		t.Errorf("legal = %+v, want none", held.legal)
	}
}

func TestTheLegalAddressesReachTheRunConfig(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
		"ALPHONE_PRIVACY_URL":  "https://example.com/legal/privacy",
		"ALPHONE_TERMS_URL":    "https://example.com/legal/terms",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := graphres.LegalLinks{
		Privacy: "https://example.com/legal/privacy",
		Terms:   "https://example.com/legal/terms",
	}
	if held.legal != want {
		t.Errorf("legal = %+v, want %+v", held.legal, want)
	}
}

func TestALegalAddressIsKeptAsWritten(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"https://example.com/legal/privacy",
		"https://example.com/legal/privacy/",
		"http://example.com/privacy",
		"https://example.com:8443/privacy",
		"https://example.com:65535/privacy",
		"https://example.com/legal?page=privacy",
		"https://example.com/legal#privacy",
		"https://example.com",
	} {
		got, err := parseLegalURL(address)

		if err != nil {
			t.Errorf("parseLegalURL(%q) error = %v, want nil", address, err)
		}
		if got != address {
			t.Errorf("parseLegalURL(%q) = %q, want it kept as written", address, got)
		}
	}
}

func TestALegalAddressRefusesWhatALinkMustNeverCarry(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"a script":          "javascript:alert(1)",
		"inline data":       "data:text/html,hello",
		"another scheme":    "ftp://example.com/privacy",
		"a mail address":    "mailto:legal@example.com",
		"a path alone":      "/legal/privacy",
		"no scheme":         "example.com/legal/privacy",
		"no host":           "https:///legal/privacy",
		"a port alone":      "https://:8443/legal/privacy",
		"a port past 65535": "https://example.com:65536/legal/privacy",
		"a zero port":       "https://example.com:0/legal/privacy",
		"a user":            "https://user@example.com/legal/privacy",
		"a user and secret": "https://user:secret@example.com/legal/privacy",
		"a space":           "https://exa mple.com/legal/privacy",
		"a control byte":    "https://example.com/legal\x7f",
	} {
		if got, err := parseLegalURL(address); err == nil {
			t.Errorf("%s: parseLegalURL(%q) = %q, want an error", name, address, got)
		}
	}
}

func TestALegalAddressNeverPrintsWhatItRefuses(t *testing.T) {
	t.Parallel()

	for _, address := range []string{
		"https://user:secret@example.com/legal/privacy",
		"https://user:secret@exa mple.com/legal/privacy",
		"ftp://user:secret@example.com/legal/privacy",
	} {
		_, err := parseLegalURL(address)

		if err == nil {
			t.Fatalf("parseLegalURL(%q) error = nil, want one", address)
		}
		if strings.Contains(err.Error(), "secret") {
			t.Errorf("parseLegalURL(%q) error = %q, want the value left out", address, err)
		}
	}
}

func TestABadLegalAddressStopsTheServer(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"ALPHONE_PRIVACY_URL", "ALPHONE_TERMS_URL"} {
		_, err := loadRunConfig(testkit.Getenv(map[string]string{
			"ALPHONE_DATABASE_URL": "postgres://localhost/x",
			key:                    "javascript:alert(1)",
		}))

		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("loadRunConfig() with a bad %s error = %v, want one naming it", key, err)
		}
	}
}

func TestABadLegalAddressLeavesTheAccountCommandsAlone(t *testing.T) {
	t.Parallel()

	_, err := loadComposeSettings(settingsEnv(testkit.Getenv(map[string]string{
		"ALPHONE_PRIVACY_URL": "javascript:alert(1)",
	})))

	if err != nil {
		t.Errorf("loadComposeSettings() error = %v, want the address read only by the server", err)
	}
}

// loadWithAnswerRoom loads an address as both legal pages under a cap leaving spare bytes beyond what they need.
func loadWithAnswerRoom(address string, spare int) error {
	_, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL":                     "postgres://localhost/x",
		"ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES": strconv.Itoa(2*answerBytes(address) + legalAnswerRoom + spare),
		"ALPHONE_PRIVACY_URL":                      address,
		"ALPHONE_TERMS_URL":                        address,
	}))
	return err
}

func TestTheLegalAddressesMustFitTheAnonymousAnswer(t *testing.T) {
	t.Parallel()

	for name, address := range map[string]string{
		"two long addresses": "https://example.com/" + strings.Repeat("a", 480),
		"no address":         "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			least := 2*answerBytes(address) + legalAnswerRoom
			want := "ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES: must be at least " + strconv.Itoa(least)
			err := loadWithAnswerRoom(address, -1)
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Errorf("loadRunConfig() error = %v, want one starting %q", err, want)
			}
			if err := loadWithAnswerRoom(address, 0); err != nil {
				t.Errorf("loadRunConfig() at the cap error = %v, want nil", err)
			}
		})
	}
}

func TestALegalAddressCountsAsTheAnswerWritesIt(t *testing.T) {
	t.Parallel()

	address := "https://example.com/?" + strings.Repeat("&", 300)
	_, err := loadRunConfig(testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL":                     "postgres://localhost/x",
		"ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES": strconv.Itoa(len(address) + legalAnswerRoom + 100),
		"ALPHONE_PRIVACY_URL":                      address,
	}))

	if err == nil || !strings.Contains(err.Error(), "ALPHONE_GRAPH_ANONYMOUS_ANSWER_MAX_BYTES") {
		t.Errorf("loadRunConfig() error = %v, want the escapes counted against the cap", err)
	}
}

func TestTheAnswerRoomHoldsTheRestOfTheBootAnswer(t *testing.T) {
	t.Parallel()

	rest, err := json.Marshal(map[string]any{"data": map[string]any{
		"locale":     "en-US",
		"legalLinks": map[string]any{"privacyUrl": nil, "termsUrl": nil},
	}})

	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	if len(rest) > legalAnswerRoom {
		t.Errorf("the rest of the boot answer takes %d bytes, want at most %d", len(rest), legalAnswerRoom)
	}
}
