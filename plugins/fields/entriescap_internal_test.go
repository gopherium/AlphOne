// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// environment returns a getenv answering only the given variables.
func environment(held map[string]string) func(string) string {
	return func(name string) string { return held[name] }
}

func TestEntriesCapAppliesTheDefaultWhenUnset(t *testing.T) {
	t.Parallel()

	held, err := entriesCap("")

	if err != nil || held != defaultEntriesMax {
		t.Errorf("entriesCap(\"\") = %d, %v, want %d, nil", held, err, defaultEntriesMax)
	}
}

func TestEntriesCapReadsAPositiveCount(t *testing.T) {
	t.Parallel()

	held, err := entriesCap("7")

	if err != nil || held != 7 {
		t.Errorf("entriesCap(\"7\") = %d, %v, want 7, nil", held, err)
	}
}

func TestEntriesCapRefusesACountItCannotHold(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"none":                 "0",
		"fewer than none":      "-1",
		"a count in words":     "many",
		"past a Postgres int4": "2147483648",
		"a fraction":           "1.5",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := entriesCap(raw)

			if err == nil || !strings.Contains(err.Error(), entriesMaxVariable) {
				t.Errorf("entriesCap(%q) error = %v, want one naming %s", raw, err, entriesMaxVariable)
			}
		})
	}
}

func TestRegisterReadsTheEntriesCap(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{Getenv: environment(map[string]string{entriesMaxVariable: "7"})})

	if p.entriesMax != 7 {
		t.Errorf("entriesMax = %d, want 7", p.entriesMax)
	}
}

func TestRegisterAppliesTheDefaultCapWithoutAnEnvironment(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{})

	if p.entriesMax != defaultEntriesMax {
		t.Errorf("entriesMax = %d, want %d", p.entriesMax, defaultEntriesMax)
	}
}

func TestRegisterRefusesAMalformedEntriesCap(t *testing.T) {
	t.Parallel()

	_, err := Register(sdk.Deps{
		DatabaseURL: unreachableURL, Getenv: environment(map[string]string{entriesMaxVariable: "many"}),
	})

	if err == nil || !strings.Contains(err.Error(), entriesMaxVariable) {
		t.Errorf("Register() error = %v, want the malformed cap named", err)
	}
}
