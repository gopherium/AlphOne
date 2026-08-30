// SPDX-License-Identifier: Elastic-2.0

package mail_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopherium/framework/mailkit"
	"github.com/gopherium/framework/mailkit/testkit"

	"github.com/gopherium/alphone/internal/mail"
)

func mailerTo(t *testing.T, sender mailkit.Sender, overrideDir string) *mail.Mailer {
	t.Helper()

	held, err := mail.New(sender, overrideDir)
	if err != nil {
		t.Fatalf("mail.New() error = %v, want nil", err)
	}
	return held
}

func TestSendInviteCarriesTheNameAndLink(t *testing.T) {
	t.Parallel()

	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, "")

	link := "https://crm.example.com/activate?token=t-123"

	err := mailer.SendInvite(t.Context(), "maria@example.com", "Maria Perez", link)

	if err != nil {
		t.Fatalf("SendInvite() error = %v, want nil", err)
	}
	if len(sender.Messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.Messages))
	}
	held := sender.Messages[0]
	if held.To != "maria@example.com" {
		t.Errorf("To = %q, want the invited address", held.To)
	}
	if held.Subject == "" {
		t.Error("Subject is empty, want the template's first line")
	}
	if strings.Contains(held.Subject, "\n") {
		t.Errorf("Subject = %q, want one line", held.Subject)
	}
	if !strings.Contains(held.Body, "Maria Perez") {
		t.Errorf("Body = %q, want the invited name", held.Body)
	}
	if !strings.Contains(held.Body, "https://crm.example.com/activate?token=t-123") {
		t.Errorf("Body = %q, want the activation link", held.Body)
	}
}

func TestSendResetCarriesTheNameAndLink(t *testing.T) {
	t.Parallel()

	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, "")

	link := "https://crm.example.com/reset-password?token=t-456"

	err := mailer.SendReset(t.Context(), "maria@example.com", "Maria Perez", link)

	if err != nil {
		t.Fatalf("SendReset() error = %v, want nil", err)
	}
	if len(sender.Messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.Messages))
	}
	held := sender.Messages[0]
	if held.To != "maria@example.com" {
		t.Errorf("To = %q, want the account's address", held.To)
	}
	if !strings.Contains(held.Body, "Maria Perez") {
		t.Errorf("Body = %q, want the account's name", held.Body)
	}
	if !strings.Contains(held.Body, "https://crm.example.com/reset-password?token=t-456") {
		t.Errorf("Body = %q, want the reset link", held.Body)
	}
}

func TestTheTwoMailsDifferFromEachOther(t *testing.T) {
	t.Parallel()

	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, "")

	if err := mailer.SendInvite(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/a"); err != nil {
		t.Fatalf("SendInvite() error = %v, want nil", err)
	}
	if err := mailer.SendReset(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/r"); err != nil {
		t.Fatalf("SendReset() error = %v, want nil", err)
	}

	if sender.Messages[0].Subject == sender.Messages[1].Subject {
		t.Errorf("both subjects are %q, want the invite and the reset to differ", sender.Messages[0].Subject)
	}
}

func TestAnOverrideDirectoryReplacesATemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	written := "Come aboard\nHello {{.Name}}, follow {{.Link}} to begin.\n"
	if err := os.WriteFile(filepath.Join(dir, "invite.tmpl"), []byte(written), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, dir)

	if err := mailer.SendInvite(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/a"); err != nil {
		t.Fatalf("SendInvite() error = %v, want nil", err)
	}

	if sender.Messages[0].Subject != "Come aboard" {
		t.Errorf("Subject = %q, want the override's first line", sender.Messages[0].Subject)
	}
	if !strings.Contains(sender.Messages[0].Body, "follow https://crm.example.com/a to begin.") {
		t.Errorf("Body = %q, want the override rendered", sender.Messages[0].Body)
	}
}

func TestAnOverrideDirectoryLeavesTheOtherTemplateEmbedded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invite.tmpl"), []byte("Come aboard\nbody\n"), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, dir)

	if err := mailer.SendReset(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/r"); err != nil {
		t.Fatalf("SendReset() error = %v, want nil", err)
	}

	if sender.Messages[0].Subject == "Come aboard" {
		t.Error("the reset mail used the invite override, want the embedded reset template")
	}
	if !strings.Contains(sender.Messages[0].Body, "Maria Perez") {
		t.Errorf("Body = %q, want the embedded reset template rendered", sender.Messages[0].Body)
	}
}

func TestAMalformedOverrideRefusesTheSendLoudly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "invite.tmpl"), []byte("Subject\n{{.Name"), 0o600); err != nil {
		t.Fatalf("write override: %v", err)
	}
	sender := &testkit.Sender{}
	mailer := mailerTo(t, sender, dir)

	err := mailer.SendInvite(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/a")

	if err == nil {
		t.Fatal("SendInvite() error = nil, want the unparsable override refused")
	}
	if len(sender.Messages) != 0 {
		t.Errorf("sent %d messages, want none when the template cannot render", len(sender.Messages))
	}
}

func TestNewRefusesAnUnreadableOverrideDirectory(t *testing.T) {
	t.Parallel()

	_, err := mail.New(&testkit.Sender{}, filepath.Join(t.TempDir(), "absent"))

	if err == nil {
		t.Error("mail.New() error = nil, want the missing directory refused")
	}
}

func TestNewRefusesAMissingSender(t *testing.T) {
	t.Parallel()

	_, err := mail.New(nil, "")

	if err == nil {
		t.Error("mail.New() error = nil, want the missing sender refused")
	}
}

func TestASendFailureSurfaces(t *testing.T) {
	t.Parallel()

	refused := errors.New("the relay refused the message")
	mailer := mailerTo(t, &testkit.Sender{Err: refused}, "")

	inviteErr := mailer.SendInvite(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/a")
	resetErr := mailer.SendReset(t.Context(), "maria@example.com", "Maria Perez", "https://crm.example.com/r")

	if !errors.Is(inviteErr, refused) {
		t.Errorf("SendInvite() error = %v, want the relay failure", inviteErr)
	}
	if !errors.Is(resetErr, refused) {
		t.Errorf("SendReset() error = %v, want the relay failure", resetErr)
	}
}

func TestTheActivationLinkLeadsToTheActivationPath(t *testing.T) {
	t.Parallel()

	held := mail.ActivationLink("https://crm.example.com", "t-123")

	if held != "https://crm.example.com/activate?token=t-123" {
		t.Errorf("ActivationLink() = %q, want the activation path under the base", held)
	}
}

func TestTheResetLinkLeadsToTheResetPath(t *testing.T) {
	t.Parallel()

	held := mail.ResetLink("https://crm.example.com", "t-456")

	if held != "https://crm.example.com/reset-password?token=t-456" {
		t.Errorf("ResetLink() = %q, want the reset path under the base", held)
	}
}

func TestALinkWithoutABaseStaysRelative(t *testing.T) {
	t.Parallel()

	if held := mail.ActivationLink("", "t-123"); held != "/activate?token=t-123" {
		t.Errorf("ActivationLink() = %q, want the bare path", held)
	}
	if held := mail.ResetLink("", "t-456"); held != "/reset-password?token=t-456" {
		t.Errorf("ResetLink() = %q, want the bare path", held)
	}
}

func TestALinkEscapesTheToken(t *testing.T) {
	t.Parallel()

	held := mail.ActivationLink("https://crm.example.com", "a b&c=d")

	if strings.Contains(held, "a b&c=d") {
		t.Errorf("ActivationLink() = %q, want the token escaped", held)
	}
	if !strings.Contains(held, "a+b%26c%3Dd") {
		t.Errorf("ActivationLink() = %q, want the unsafe characters percent encoded", held)
	}
}
