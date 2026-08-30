// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gopherium/gouncer/authkit"
)

func TestMailIsOffWhenNoHostIsNamed(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.mail.host != "" {
		t.Errorf("mail.host = %q, want empty", held.mail.host)
	}
}

func TestTheMailSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":      "postgres://localhost/x",
		"ALPHONE_SMTP_HOST":         "mail.example.com",
		"ALPHONE_SMTP_PORT":         "2525",
		"ALPHONE_SMTP_USERNAME":     "crm",
		"ALPHONE_SMTP_PASSWORD":     "correct horse battery",
		"ALPHONE_SMTP_FROM":         "crm@example.com",
		"ALPHONE_SMTP_TLS":          "none",
		"ALPHONE_PUBLIC_URL":        "https://crm.example.com",
		"ALPHONE_MAIL_TEMPLATE_DIR": "/srv/mail",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := mailSettings{
		host:        "mail.example.com",
		port:        2525,
		username:    "crm",
		password:    "correct horse battery",
		from:        "crm@example.com",
		tls:         "none",
		publicURL:   "https://crm.example.com",
		templateDir: "/srv/mail",
	}
	if held.mail != want {
		t.Errorf("mail = %+v, want %+v", held.mail, want)
	}
}

func TestTheMailDefaultsApply(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
		"ALPHONE_SMTP_HOST":    "mail.example.com",
		"ALPHONE_SMTP_FROM":    "crm@example.com",
		"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.mail.port != 587 {
		t.Errorf("mail.port = %d, want the submission default 587", held.mail.port)
	}
	if held.mail.tls != "mandatory" {
		t.Errorf("mail.tls = %q, want mandatory", held.mail.tls)
	}
}

func TestTheTokenLifetimesDefaultWithoutAMailer(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.inviteTTL != authkit.DefaultInviteTTL {
		t.Errorf("inviteTTL = %v, want the authkit default %v", held.inviteTTL, authkit.DefaultInviteTTL)
	}
	if held.resetTTL != authkit.DefaultResetTTL {
		t.Errorf("resetTTL = %v, want the authkit default %v", held.resetTTL, authkit.DefaultResetTTL)
	}
}

func TestTheTokenLifetimesAreReadWithoutAMailer(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
		"ALPHONE_INVITE_TTL":   "24h",
		"ALPHONE_RESET_TTL":    "30m",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.inviteTTL != 24*time.Hour {
		t.Errorf("inviteTTL = %v, want 24h", held.inviteTTL)
	}
	if held.resetTTL != 30*time.Minute {
		t.Errorf("resetTTL = %v, want 30m", held.resetTTL)
	}
}

func TestTheTokenLifetimesRefuseAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"an unreadable invite lifetime": {"ALPHONE_INVITE_TTL": "a week"},
		"a zero invite lifetime":        {"ALPHONE_INVITE_TTL": "0"},
		"a negative invite lifetime":    {"ALPHONE_INVITE_TTL": "-1h"},
		"an unreadable reset lifetime":  {"ALPHONE_RESET_TTL": "soon"},
		"a zero reset lifetime":         {"ALPHONE_RESET_TTL": "0s"},
		"a negative reset lifetime":     {"ALPHONE_RESET_TTL": "-5m"},
	}
	for testName, vars := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			vars["ALPHONE_DATABASE_URL"] = "postgres://localhost/x"

			_, err := loadRunConfig(testGetenv(vars))

			if err == nil {
				t.Error("loadRunConfig() error = nil, want the lifetime refused")
			}
		})
	}
}

func TestAMailerNamesItsRequiredCompanions(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		unset string
	}{
		"the sender address": {unset: "ALPHONE_SMTP_FROM"},
		"the public url":     {unset: "ALPHONE_PUBLIC_URL"},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			vars := map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				"ALPHONE_SMTP_HOST":    "mail.example.com",
				"ALPHONE_SMTP_FROM":    "crm@example.com",
				"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
			}
			delete(vars, tt.unset)

			_, err := loadRunConfig(testGetenv(vars))

			if err == nil {
				t.Fatalf("loadRunConfig() without %s error = nil, want it required", tt.unset)
			}
			if !strings.Contains(err.Error(), tt.unset) {
				t.Errorf("error %q does not name %s", err, tt.unset)
			}
		})
	}
}

func TestMailSettingsWithoutAHostAreRefused(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"a stray port":     {"ALPHONE_SMTP_PORT": "2525"},
		"a stray username": {"ALPHONE_SMTP_USERNAME": "crm"},
		"a stray password": {"ALPHONE_SMTP_PASSWORD": "correct horse battery"},
		"a stray sender":   {"ALPHONE_SMTP_FROM": "crm@example.com"},
		"a stray policy":   {"ALPHONE_SMTP_TLS": "none"},
	}
	for testName, vars := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			vars["ALPHONE_DATABASE_URL"] = "postgres://localhost/x"

			_, err := loadRunConfig(testGetenv(vars))

			if err == nil {
				t.Error("loadRunConfig() error = nil, want the hostless setting refused")
			}
			if err != nil && !strings.Contains(err.Error(), "ALPHONE_SMTP_HOST") {
				t.Errorf("error %q does not point at ALPHONE_SMTP_HOST", err)
			}
		})
	}
}

func TestTheMailPortRefusesAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"not a number":     "the submission port",
		"zero":             "0",
		"negative":         "-25",
		"beyond the range": "70000",
	}
	for testName, raw := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				"ALPHONE_SMTP_HOST":    "mail.example.com",
				"ALPHONE_SMTP_FROM":    "crm@example.com",
				"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
				"ALPHONE_SMTP_PORT":    raw,
			}))

			if err == nil {
				t.Errorf("loadRunConfig() with port %q error = nil, want it refused", raw)
			}
		})
	}
}

func TestTheTransportSecurityAcceptsEveryNamedPolicy(t *testing.T) {
	t.Parallel()

	for _, policy := range []string{"mandatory", "opportunistic", "none"} {
		t.Run(policy, func(t *testing.T) {
			t.Parallel()

			held, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				"ALPHONE_SMTP_HOST":    "mail.example.com",
				"ALPHONE_SMTP_FROM":    "crm@example.com",
				"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
				"ALPHONE_SMTP_TLS":     policy,
			}))

			if err != nil {
				t.Fatalf("loadRunConfig() error = %v, want nil", err)
			}
			if held.mail.tls != policy {
				t.Errorf("mail.tls = %q, want %q", held.mail.tls, policy)
			}
		})
	}
}

func TestTheTransportSecurityRefusesAnUnknownPolicy(t *testing.T) {
	t.Parallel()

	_, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
		"ALPHONE_SMTP_HOST":    "mail.example.com",
		"ALPHONE_SMTP_FROM":    "crm@example.com",
		"ALPHONE_PUBLIC_URL":   "https://crm.example.com",
		"ALPHONE_SMTP_TLS":     "tls13",
	}))

	if err == nil {
		t.Error("loadRunConfig() error = nil, want the policy refused")
	}
}

func TestThePublicURLRefusesAMalformedValue(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"no scheme":         "crm.example.com",
		"a bare path":       "/crm",
		"an unknown scheme": "ftp://crm.example.com",
	}
	for testName, raw := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				"ALPHONE_SMTP_HOST":    "mail.example.com",
				"ALPHONE_SMTP_FROM":    "crm@example.com",
				"ALPHONE_PUBLIC_URL":   raw,
			}))

			if err == nil {
				t.Errorf("loadRunConfig() with public url %q error = nil, want it refused", raw)
			}
		})
	}
}

func TestThePublicURLDropsItsTrailingSlash(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
		"ALPHONE_SMTP_HOST":    "mail.example.com",
		"ALPHONE_SMTP_FROM":    "crm@example.com",
		"ALPHONE_PUBLIC_URL":   "https://crm.example.com/",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.mail.publicURL != "https://crm.example.com" {
		t.Errorf("mail.publicURL = %q, want the slash dropped", held.mail.publicURL)
	}
}
