// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/gopherium/alphone/sdk"
)

var errDepsRecorded = errors.New("the dependencies were recorded")

// recordedPublicURL boots run with env, answering the public URL the host handed the plugins.
func recordedPublicURL(t *testing.T, env map[string]string) string {
	t.Helper()
	env["ALPHONE_DATABASE_URL"] = testDatabaseURL(t)
	var handed string
	recording := func(deps sdk.Deps) ([]sdk.Plugin, error) {
		handed = deps.PublicURL
		return nil, errDepsRecorded
	}

	err := run(t.Context(), testGetenv(env), io.Discard, recording)

	if !errors.Is(err, errDepsRecorded) {
		t.Fatalf("run() error = %v, want the recording plugin's own refusal", err)
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

func TestRunHandsAnEmptyPublicURLWithoutARelay(t *testing.T) {
	t.Parallel()

	handed := recordedPublicURL(t, map[string]string{})

	if strings.TrimSpace(handed) != "" {
		t.Errorf("plugins received public URL %q, want none without a relay", handed)
	}
}
