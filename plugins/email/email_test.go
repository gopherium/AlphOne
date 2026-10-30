// SPDX-License-Identifier: Elastic-2.0

package email_test

import (
	"testing"

	"github.com/gopherium/alphone/plugins/email"
	"github.com/gopherium/alphone/sdk"
)

func TestIDIsEmail(t *testing.T) {
	t.Parallel()

	p := newPlugin(t)

	if got := p.ID(); got != "email" {
		t.Errorf("ID() = %q, want %q", got, "email")
	}
}

func TestRegisterAcceptsEmptyDeps(t *testing.T) {
	t.Parallel()

	p, err := email.Register(sdk.Deps{})

	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if p == nil {
		t.Fatal("Register() = nil, want a plugin")
	}
}

func TestStartAndStopAnswerNil(t *testing.T) {
	t.Parallel()

	p := newPlugin(t)

	if err := p.Start(t.Context()); err != nil {
		t.Errorf("Start() error = %v, want nil", err)
	}
	if err := p.Stop(t.Context()); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestThePluginTakesNoOptionalPart(t *testing.T) {
	t.Parallel()

	var p sdk.Plugin = newPlugin(t)
	parts := []struct {
		name  string
		takes bool
	}{
		{"Migrator", implements[sdk.Migrator](p)},
		{"Seeder", implements[sdk.Seeder](p)},
		{"RouteProvider", implements[sdk.RouteProvider](p)},
		{"PublicPathProvider", implements[sdk.PublicPathProvider](p)},
		{"CommandProvider", implements[sdk.CommandProvider](p)},
		{"EventProvider", implements[sdk.EventProvider](p)},
		{"AreaProvider", implements[sdk.AreaProvider](p)},
		{"RoleProvider", implements[sdk.RoleProvider](p)},
		{"TopRoleProvider", implements[sdk.TopRoleProvider](p)},
		{"FieldSource", implements[sdk.FieldSource](p)},
		{"FieldProvider", implements[sdk.FieldProvider](p)},
		{"FieldConsumer", implements[sdk.FieldConsumer](p)},
		{"CredentialProvider", implements[sdk.CredentialProvider](p)},
		{"CredentialConsumer", implements[sdk.CredentialConsumer](p)},
		{"MailSenderConsumer", implements[sdk.MailSenderConsumer](p)},
		{"TenantGateConsumer", implements[sdk.TenantGateConsumer](p)},
		{"RoleCheckerConsumer", implements[sdk.RoleCheckerConsumer](p)},
		{"SettingStoreConsumer", implements[sdk.SettingStoreConsumer](p)},
		{"EncrypterConsumer", implements[sdk.EncrypterConsumer](p)},
		{"FileStoreConsumer", implements[sdk.FileStoreConsumer](p)},
	}

	for _, part := range parts {
		if part.takes {
			t.Errorf("the plugin implements sdk.%s, want an inert plugin", part.name)
		}
	}
}

// implements reports whether p satisfies the interface T.
func implements[T any](p sdk.Plugin) bool {
	_, ok := p.(T)
	return ok
}

// newPlugin registers the email plugin on empty deps.
func newPlugin(t *testing.T) *email.Plugin {
	t.Helper()
	p, err := email.Register(sdk.Deps{})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	return p
}
