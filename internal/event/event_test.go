// SPDX-License-Identifier: Elastic-2.0

package event_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
)

func TestNewStampsAnIdentifierAndTime(t *testing.T) {
	t.Parallel()

	created, err := event.New(event.TaskCreated, map[string]any{"id": "abc"})

	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if created.ID == uuid.Nil {
		t.Error("ID = zero, want a generated identifier")
	}
	if created.OccurredAt.IsZero() {
		t.Error("OccurredAt = zero, want the publication time")
	}
	if created.Name != event.TaskCreated {
		t.Errorf("Name = %q, want %q", created.Name, event.TaskCreated)
	}
}

func TestNewRejectsAnUnpublishedName(t *testing.T) {
	t.Parallel()

	_, err := event.New("task.deleted", nil)

	if !errors.Is(err, event.ErrUnknownName) {
		t.Errorf("New() error = %v, want %v", err, event.ErrUnknownName)
	}
}

func TestNamesListsEveryPublishedEvent(t *testing.T) {
	t.Parallel()

	names := event.Names()

	want := []event.Name{
		event.TaskCreated,
		event.TaskCompleted,
		event.ContactCreated,
		event.WhatsAppMessageReceived,
	}
	for _, name := range want {
		if !slices.Contains(names, name) {
			t.Errorf("Names() is missing %q", name)
		}
	}
	if len(names) != len(want) {
		t.Errorf("Names() has %d entries, want %d, the event budget is deliberate", len(names), len(want))
	}
}

func TestNamesCannotBeMutatedByItsCaller(t *testing.T) {
	t.Parallel()

	names := event.Names()
	names[0] = "task.deleted"

	if slices.Contains(event.Names(), event.Name("task.deleted")) {
		t.Error("a caller rewrote the published event list")
	}
	if !slices.Contains(event.Names(), event.TaskCreated) {
		t.Error("a caller removed a published event")
	}
}

func TestValidAcceptsOnlyPublishedNames(t *testing.T) {
	t.Parallel()

	if !event.TaskCreated.Valid() {
		t.Error("TaskCreated.Valid() = false, want true")
	}
	if event.Name("task.deleted").Valid() {
		t.Error("unpublished name reported valid")
	}
}

func TestPayloadCarriesTheEnvelope(t *testing.T) {
	t.Parallel()

	created, err := event.New(event.TaskCreated, map[string]any{"id": "abc", "title": "Call Maria"})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	body, err := created.Payload()

	if err != nil {
		t.Fatalf("Payload() error = %v, want nil", err)
	}
	var envelope struct {
		ID         string         `json:"id"`
		Event      string         `json:"event"`
		OccurredAt string         `json:"occurred_at"`
		Data       map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("unmarshalling the payload: %v", err)
	}
	if envelope.ID != created.ID.String() {
		t.Errorf("id = %q, want the event id %q", envelope.ID, created.ID)
	}
	if envelope.Event != "task.created" {
		t.Errorf("event = %q, want %q", envelope.Event, "task.created")
	}
	if envelope.OccurredAt == "" {
		t.Error("occurred_at is empty, want an RFC 3339 timestamp")
	}
	if envelope.Data["title"] != "Call Maria" {
		t.Errorf("data.title = %v, want the published field", envelope.Data["title"])
	}
}

func TestPayloadIsStableForTheSameEvent(t *testing.T) {
	t.Parallel()

	created, err := event.New(event.ContactCreated, map[string]any{"b": 2, "a": 1})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	first, err := created.Payload()
	if err != nil {
		t.Fatalf("Payload() error = %v, want nil", err)
	}
	second, err := created.Payload()
	if err != nil {
		t.Fatalf("Payload() error = %v, want nil", err)
	}

	if string(first) != string(second) {
		t.Errorf("Payload() is unstable, a signature would not verify:\n%s\n%s", first, second)
	}
}

func TestPayloadReportsUnencodableData(t *testing.T) {
	t.Parallel()

	created, err := event.New(event.TaskCreated, map[string]any{"ch": make(chan int)})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if _, err := created.Payload(); err == nil {
		t.Error("Payload() error = nil, want the encoding failure reported")
	}
}
