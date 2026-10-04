// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

// settings returns a reader under the program prefix answering only the given variables.
func settings(held map[string]string) sdk.Env {
	return sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string { return held[name] }}
}

func TestPaddedSettingsRegisterLikePlainOnes(t *testing.T) {
	t.Parallel()

	for name, value := range map[string]string{"plain": "7", "padded": "  7  "} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			p := newUnreachablePlugin(t, sdk.Deps{Env: settings(map[string]string{"ALPHONE_FIELDS_ENTRIES_MAX": value})})

			if p.entriesMax != 7 {
				t.Errorf("entriesMax over %q = %d, want 7 read through the settings reader", value, p.entriesMax)
			}
		})
	}
}

func TestRegisterAppliesTheDefaultCapWithoutAnEnvironment(t *testing.T) {
	t.Parallel()

	p := newUnreachablePlugin(t, sdk.Deps{})

	if p.entriesMax != defaultEntriesMax {
		t.Errorf("entriesMax = %d, want %d", p.entriesMax, defaultEntriesMax)
	}
}

func TestRegisterRefusesAnEntriesCapItCannotHold(t *testing.T) {
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

			_, err := Register(sdk.Deps{
				DatabaseURL: unreachableURL, Env: settings(map[string]string{"ALPHONE_FIELDS_ENTRIES_MAX": raw}),
			})

			if err == nil || !strings.HasPrefix(err.Error(), "ALPHONE_FIELDS_ENTRIES_MAX: must ") {
				t.Errorf("Register() over %q error = %v, want the cap refused by name", raw, err)
			}
		})
	}
}
