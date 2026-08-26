// SPDX-License-Identifier: Elastic-2.0

package whatsapp_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

type recordingPublisher struct {
	mu        sync.Mutex
	published []string
	data      []map[string]any
}

// Publish records the event name and its data.
func (p *recordingPublisher) Publish(_ context.Context, name string, data map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.published = append(p.published, name)
	p.data = append(p.data, data)
}

// names returns a copy of the event names recorded so far.
func (p *recordingPublisher) names() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.published...)
}

// newPublishingPlugin returns a plugin announcing to the returned recorder.
func newPublishingPlugin(t *testing.T) (*whatsapp.Plugin, *recordingPublisher) {
	t.Helper()
	cfg := newTestDatabase(t)
	pool := newAssertionPool(t, cfg.URL())
	publisher := &recordingPublisher{}
	p, err := whatsapp.Register(sdk.Deps{
		DatabaseURL: cfg.URL(),
		Resolver:    resolverBridge{resolver: contact.NewResolver(postgres.NewContactStore(pool))},
		Getenv: func(key string) string {
			return map[string]string{
				"ALPHONE_WHATSAPP_APP_SECRET":      "app-secret",
				"ALPHONE_WHATSAPP_PHONE_NUMBER_ID": "555000111",
			}[key]
		},
		Events: publisher,
	})
	if err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	t.Cleanup(func() { _ = p.Stop(context.Background()) })
	if err := p.Migrate(t.Context()); err != nil {
		t.Fatalf("Migrate() error = %v, want nil", err)
	}
	return p, publisher
}

func TestInboundMessagePublishesItsArrival(t *testing.T) {
	t.Parallel()

	p, publisher := newPublishingPlugin(t)
	body := eventBody("wamid.publish1", "184467235", "Maria Perez", "1751791000", "Hello there")

	recorder := postEvent(t, p.Routes(), sign("app-secret", body), body)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusOK, recorder.Body)
	}
	names := publisher.names()
	if len(names) != 1 || names[0] != "whatsapp.message.received" {
		t.Fatalf("published %v, want one whatsapp.message.received", names)
	}
	if publisher.data[0]["contact_id"] == nil {
		t.Error("data.contact_id is absent, want the resolved contact")
	}
	if publisher.data[0]["text"] != "Hello there" {
		t.Errorf("data.text = %v, want the message body", publisher.data[0]["text"])
	}
}

func TestARepeatedInboundMessagePublishesOnce(t *testing.T) {
	t.Parallel()

	p, publisher := newPublishingPlugin(t)
	body := eventBody("wamid.publish2", "184467235", "Maria Perez", "1751791000", "Hello there")
	for range 2 {
		if recorder := postEvent(t, p.Routes(), sign("app-secret", body), body); recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
		}
	}

	if names := publisher.names(); len(names) != 1 {
		t.Errorf("published %v for a duplicate delivery, want one", names)
	}
}
