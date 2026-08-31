// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// discardLogger returns a logger writing nowhere.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestTheHostRefusesAMailSenderItCannotBuild(t *testing.T) {
	t.Parallel()

	tests := map[string]mailSettings{
		"an unparseable sender address": {
			host: "mail.example.com",
			port: 2525,
			from: "not an address",
			tls:  "none",
		},
		"an empty sender address": {
			host: "mail.example.com",
			port: 2525,
			tls:  "none",
		},
		"a password without a username": {
			host:     "mail.example.com",
			port:     2525,
			password: "correct horse battery",
			from:     "crm@example.com",
			tls:      "none",
		},
		"an unknown transport policy": {
			host: "mail.example.com",
			port: 2525,
			from: "crm@example.com",
			tls:  "tls13",
		},
	}
	for testName, refusedSettings := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			held, err := buildMailSender(refusedSettings, discardLogger())

			if err == nil {
				t.Fatal("buildMailSender() error = nil, want the relay settings refused")
			}
			if !strings.Contains(err.Error(), "build mail sender") {
				t.Errorf("buildMailSender() error = %v, want it named as a sender build failure", err)
			}
			if held != nil {
				t.Errorf("buildMailSender() = %#v, want nothing on failure", held)
			}
		})
	}
}

func TestBuildingMailStopsAtARefusedSender(t *testing.T) {
	t.Parallel()

	sender, mailer, err := buildMail(mailSettings{
		host:      "mail.example.com",
		port:      2525,
		from:      "not an address",
		tls:       "none",
		publicURL: "https://crm.example.com",
	}, discardLogger())

	if err == nil {
		t.Fatal("buildMail() error = nil, want the refused sender surfaced")
	}
	if !strings.Contains(err.Error(), "build mail sender") {
		t.Errorf("buildMail() error = %v, want the sender failure surfaced", err)
	}
	if sender != nil || mailer != nil {
		t.Errorf("buildMail() = %#v, %#v, want nothing on failure", sender, mailer)
	}
}

func TestBuildingMailStopsAtARefusedMailer(t *testing.T) {
	t.Parallel()

	absentTemplateDir := filepath.Join(t.TempDir(), "absent")

	sender, mailer, err := buildMail(mailSettings{
		host:        "mail.example.com",
		port:        2525,
		from:        "crm@example.com",
		tls:         "none",
		publicURL:   "https://crm.example.com",
		templateDir: absentTemplateDir,
	}, discardLogger())

	if err == nil {
		t.Fatal("buildMail() error = nil, want the unusable template directory refused")
	}
	if !strings.Contains(err.Error(), absentTemplateDir) {
		t.Errorf("buildMail() error = %v, want it to name %s", err, absentTemplateDir)
	}
	if sender != nil || mailer != nil {
		t.Errorf("buildMail() = %#v, %#v, want nothing on failure", sender, mailer)
	}
}

func TestTheGreetingIsEmptyForAnUnparseableAddress(t *testing.T) {
	t.Parallel()

	const unparseablePublicURL = "https://crm.example.com/\x7f"

	held := heloFor(unparseablePublicURL)

	if held != "" {
		t.Errorf("heloFor(%q) = %q, want empty", unparseablePublicURL, held)
	}
}

func TestTheTemplateSourceNamesTheOverrideDirectory(t *testing.T) {
	t.Parallel()

	held := templateSource("/srv/mail")

	if held != "/srv/mail" {
		t.Errorf("templateSource() = %q, want /srv/mail", held)
	}
}

func TestThePublicURLRefusesAnUnparseableValue(t *testing.T) {
	t.Parallel()

	const unparseablePublicURL = "https://crm.example.com/\x7f"

	held, err := parsePublicURL(unparseablePublicURL)

	if err == nil {
		t.Fatal("parsePublicURL() error = nil, want the unparseable address refused")
	}
	if !strings.Contains(err.Error(), "ALPHONE_PUBLIC_URL") {
		t.Errorf("parsePublicURL() error = %v, want it to name ALPHONE_PUBLIC_URL", err)
	}
	if held != "" {
		t.Errorf("parsePublicURL() = %q, want empty on failure", held)
	}
}

func TestRunStopsWhenTheMailRelayIsRefused(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	databaseURL := testDatabaseURL(t)

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": databaseURL,
		"ALPHONE_ADDR":         freeAddr(t),
		"ALPHONE_SMTP_HOST":    "mail.example.com",
		"ALPHONE_SMTP_PORT":    "2525",
		"ALPHONE_SMTP_FROM":    "not an address",
		"ALPHONE_SMTP_TLS":     "none",
		"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
	}), io.Discard, registerPlugins)

	if err == nil {
		t.Fatal("run() error = nil, want the refused mail relay surfaced")
	}
	if !strings.Contains(err.Error(), "build mail sender") {
		t.Errorf("run() error = %v, want the mail relay failure", err)
	}
}
