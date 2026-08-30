// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/gopherium/framework/mailkit"

	"github.com/gopherium/alphone/sdk"
)

// mailTakingPlugin records the mail sender the host hands it.
type mailTakingPlugin struct {
	inertPlugin
	received sdk.MailSender
	handed   bool
}

// UseMailSender keeps the handed sender.
func (p *mailTakingPlugin) UseMailSender(sender sdk.MailSender) {
	p.received = sender
	p.handed = true
}

// countingSender records the mailkit messages a bridge passes down.
type countingSender struct {
	messages []mailkit.Message
	err      error
}

// Send keeps m or answers the configured failure.
func (s *countingSender) Send(_ context.Context, m mailkit.Message) error {
	if s.err != nil {
		return s.err
	}
	s.messages = append(s.messages, m)
	return nil
}

func TestWiringHandsTheMailSenderToEveryConsumer(t *testing.T) {
	t.Parallel()

	taking := &mailTakingPlugin{inertPlugin: inertPlugin{id: "whatsapp"}}
	bridge := mailSenderBridge{sender: &countingSender{}}

	wireMailSender([]sdk.Plugin{taking, inertPlugin{id: "bystander"}}, bridge)

	if !taking.handed {
		t.Fatal("the plugin received no sender, want the host's")
	}
	if taking.received != sdk.MailSender(bridge) {
		t.Errorf("the plugin received %#v, want the very sender the host handed it", taking.received)
	}
}

func TestWiringHandsNoMailSenderWithoutARelay(t *testing.T) {
	t.Parallel()

	taking := &mailTakingPlugin{inertPlugin: inertPlugin{id: "whatsapp"}}

	wireMailSenderFrom([]sdk.Plugin{taking}, nil)

	if taking.handed {
		t.Errorf("the plugin received %#v, want nothing handed without a relay", taking.received)
	}
}

func TestTheMailSenderBridgeCarriesTheMessageDown(t *testing.T) {
	t.Parallel()

	sender := &countingSender{}
	bridge := mailSenderBridge{sender: sender}

	err := bridge.Send(t.Context(), sdk.Mail{
		To:      "maria@example.com",
		Subject: "Your workspace",
		Body:    "Hello Maria Perez",
	})

	if err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.messages))
	}
	held := sender.messages[0]
	want := mailkit.Message{To: "maria@example.com", Subject: "Your workspace", Body: "Hello Maria Perez"}
	if held != want {
		t.Errorf("carried %+v, want %+v", held, want)
	}
}

func TestTheMailSenderBridgeSurfacesAFailure(t *testing.T) {
	t.Parallel()

	refused := errors.New("the relay refused the message")
	bridge := mailSenderBridge{sender: &countingSender{err: refused}}

	err := bridge.Send(t.Context(), sdk.Mail{To: "maria@example.com", Subject: "s", Body: "b"})

	if !errors.Is(err, refused) {
		t.Errorf("Send() error = %v, want the relay failure", err)
	}
}

func TestTheHostBuildsNoMailSenderWithoutARelay(t *testing.T) {
	t.Parallel()

	held, err := buildMailSender(mailSettings{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err != nil {
		t.Fatalf("buildMailSender() error = %v, want nil", err)
	}
	if held != nil {
		t.Errorf("buildMailSender() = %#v, want nothing without a relay", held)
	}
}
