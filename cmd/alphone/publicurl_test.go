// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/sdk"
)

var errDepsRecorded = errors.New("the dependencies were recorded")

// refusedPublicURL is a public address on a scheme AlphOne refuses.
const refusedPublicURL = "ftp://crm.example.com"

// refusedPublicURLError is the error a public address on that scheme meets.
const refusedPublicURLError = `ALPHONE_PUBLIC_URL: must be an http or https address, got "ftp://crm.example.com"`

// recordedPublicURL boots run with env, answering the public URL the host handed the plugins.
func recordedPublicURL(t *testing.T, env map[string]string) string {
	t.Helper()
	env["ALPHONE_DATABASE_URL"] = testDatabaseURL(t)
	var handed string
	recording := func(deps sdk.Deps) ([]sdk.Plugin, error) {
		handed = deps.PublicURL
		return nil, errDepsRecorded
	}

	err := run(t.Context(), testkit.Getenv(env), io.Discard, recording)

	if !errors.Is(err, errDepsRecorded) {
		t.Fatalf("run() error = %v, want the recording plugin's own error", err)
	}
	return handed
}

func TestRunHandsThePublicURLToPlugins(t *testing.T) {
	t.Parallel()

	handed := recordedPublicURL(t, map[string]string{
		"ALPHONE_SMTP_HOST":  "127.0.0.1",
		"ALPHONE_SMTP_PORT":  "2525",
		"ALPHONE_SMTP_FROM":  "crm@example.com",
		"ALPHONE_SMTP_TLS":   "none",
		"ALPHONE_PUBLIC_URL": "https://crm.example.com",
	})

	if handed != "https://crm.example.com" {
		t.Errorf("plugins received public URL %q, want the configured address", handed)
	}
}

func TestRunHandsThePublicURLToPluginsWithoutARelay(t *testing.T) {
	t.Parallel()

	handed := recordedPublicURL(t, map[string]string{"ALPHONE_PUBLIC_URL": "https://crm.example.com"})

	if handed != "https://crm.example.com" {
		t.Errorf("plugins received public URL %q, want the configured address without a relay", handed)
	}
}

func TestRunHandsAnEmptyPublicURLWhenNoneIsSet(t *testing.T) {
	t.Parallel()

	handed := recordedPublicURL(t, map[string]string{})

	if handed != "" {
		t.Errorf("plugins received public URL %q, want none when no address is set", handed)
	}
}

func TestRunRefusesAMalformedPublicURLWithoutARelay(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL": unreachableDatabaseURL,
		"ALPHONE_PUBLIC_URL":   refusedPublicURL,
	}), io.Discard, registeringNothing)

	if err == nil || err.Error() != refusedPublicURLError {
		t.Errorf("run() error = %v, want %q", err, refusedPublicURLError)
	}
}

func TestTheCommandsRefuseAMalformedPublicURLWithoutARelay(t *testing.T) {
	t.Parallel()

	lines := map[string][]string{
		"check":              {"check"},
		"account:grant-role": {"account:grant-role", "-role", "admin", "-yes", "-as", "admin@example.com"},
		"account:role":       {"account:role", "maria.perez@example.com", "admin", "-yes", "-as", "admin@example.com"},
		"account:disable":    {"account:disable", "maria.perez@example.com", "-yes", "-as", "admin@example.com"},
		"account:enable":     {"account:enable", "maria.perez@example.com", "-yes", "-as", "admin@example.com"},
	}
	for name, args := range lines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(map[string]string{
				"ALPHONE_DATABASE_URL": unreachableDatabaseURL,
				"ALPHONE_PUBLIC_URL":   refusedPublicURL,
			}), "", args...)

			if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, refusedPublicURLError) {
				t.Errorf("%s = %d with stderr %q, want 1 and %q", name, got.Code, got.Stderr, refusedPublicURLError)
			}
		})
	}
}
