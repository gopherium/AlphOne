// SPDX-License-Identifier: Elastic-2.0

package webhook_test

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
)

var errDispatchBackend = errors.New("dispatch backend unavailable")

type fakeDispatchStore struct {
	mu        sync.Mutex
	subs      []webhook.Subscription
	queued    []webhook.Delivery
	listErr   error
	enqueueAt map[uuid.UUID]error
}

func newFakeDispatchStore(subs ...webhook.Subscription) *fakeDispatchStore {
	return &fakeDispatchStore{subs: subs, enqueueAt: map[uuid.UUID]error{}}
}

func (s *fakeDispatchStore) ListSubscriptionsForEvent(
	_ context.Context,
	name event.Name,
) ([]webhook.Subscription, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	var matching []webhook.Subscription
	for _, sub := range s.subs {
		if sub.Wants(name) {
			matching = append(matching, sub)
		}
	}
	return matching, nil
}

func (s *fakeDispatchStore) EnqueueDelivery(_ context.Context, d webhook.Delivery) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.enqueueAt[d.SubscriptionID]; err != nil {
		return err
	}
	s.queued = append(s.queued, d)
	return nil
}

// newDispatcher returns a dispatcher over store, and the log it writes to.
func newDispatcher(store *fakeDispatchStore) (*webhook.Dispatcher, *strings.Builder) {
	var logged strings.Builder
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	return webhook.NewDispatcher(store, logger), &logged
}

func mustSub(t *testing.T, url string, events ...event.Name) webhook.Subscription {
	t.Helper()
	sub, err := webhook.NewSubscription(uuid.Must(uuid.NewV7()), url, events)
	if err != nil {
		t.Fatalf("NewSubscription() error = %v, want nil", err)
	}
	return sub
}

func TestDispatcherQueuesOneDeliveryPerMatchingSubscription(t *testing.T) {
	t.Parallel()

	wanted := mustSub(t, "https://example.com/a", event.TaskCreated)
	also := mustSub(t, "https://example.com/b", event.TaskCreated, event.ContactCreated)
	other := mustSub(t, "https://example.com/c", event.ContactCreated)
	store := newFakeDispatchStore(wanted, also, other)
	dispatcher, _ := newDispatcher(store)

	dispatcher.Publish(t.Context(), event.TaskCreated, map[string]any{"title": "Call Maria"})

	if len(store.queued) != 2 {
		t.Fatalf("queued %d deliveries, want 2", len(store.queued))
	}
	for _, queued := range store.queued {
		if queued.EventName != event.TaskCreated {
			t.Errorf("EventName = %q, want %q", queued.EventName, event.TaskCreated)
		}
		if !strings.Contains(string(queued.Payload), "Call Maria") {
			t.Errorf("Payload = %s, want the published data", queued.Payload)
		}
		if queued.SubscriptionID == other.ID {
			t.Error("queued a delivery for a subscription that did not want the event")
		}
	}
}

func TestDispatcherQueuesNothingWithoutASubscriber(t *testing.T) {
	t.Parallel()

	store := newFakeDispatchStore(mustSub(t, "https://example.com/a", event.ContactCreated))
	dispatcher, _ := newDispatcher(store)

	dispatcher.Publish(t.Context(), event.TaskCreated, nil)

	if len(store.queued) != 0 {
		t.Errorf("queued %d deliveries, want none", len(store.queued))
	}
}

func TestDispatcherRefusesAnUnpublishedName(t *testing.T) {
	t.Parallel()

	store := newFakeDispatchStore(mustSub(t, "https://example.com/a", event.TaskCreated))
	dispatcher, logged := newDispatcher(store)

	dispatcher.Publish(t.Context(), "task.deleted", nil)

	if len(store.queued) != 0 {
		t.Errorf("queued %d deliveries for an unpublished event, want none", len(store.queued))
	}
	if !strings.Contains(logged.String(), "task.deleted") {
		t.Errorf("log = %q, want the refused event named", logged.String())
	}
}

func TestDispatcherSurvivesAListingFailure(t *testing.T) {
	t.Parallel()

	store := newFakeDispatchStore()
	store.listErr = errDispatchBackend
	dispatcher, logged := newDispatcher(store)

	dispatcher.Publish(t.Context(), event.TaskCreated, nil)

	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}

func TestDispatcherKeepsGoingAfterOneSubscriberFailsToQueue(t *testing.T) {
	t.Parallel()

	broken := mustSub(t, "https://example.com/broken", event.TaskCreated)
	healthy := mustSub(t, "https://example.com/healthy", event.TaskCreated)
	store := newFakeDispatchStore(broken, healthy)
	store.enqueueAt[broken.ID] = errDispatchBackend
	dispatcher, logged := newDispatcher(store)

	dispatcher.Publish(t.Context(), event.TaskCreated, nil)

	if len(store.queued) != 1 || store.queued[0].SubscriptionID != healthy.ID {
		t.Errorf("queued %d deliveries, want the healthy subscriber still served", len(store.queued))
	}
	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}

func TestDispatcherRefusesUnencodableData(t *testing.T) {
	t.Parallel()

	store := newFakeDispatchStore(mustSub(t, "https://example.com/a", event.TaskCreated))
	dispatcher, logged := newDispatcher(store)

	dispatcher.Publish(t.Context(), event.TaskCreated, map[string]any{"ch": make(chan int)})

	if len(store.queued) != 0 {
		t.Errorf("queued %d deliveries for unencodable data, want none", len(store.queued))
	}
	if !strings.Contains(logged.String(), "level=ERROR") {
		t.Errorf("log = %q, want the failure recorded", logged.String())
	}
}
