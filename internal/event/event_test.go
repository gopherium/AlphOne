// SPDX-License-Identifier: Elastic-2.0

package event_test

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
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

func TestNamesListsTheCoreEventsFirst(t *testing.T) {
	t.Parallel()

	names := event.Names()

	want := []event.Name{event.TaskCreated, event.TaskCompleted, event.ContactCreated}
	if len(names) < len(want) || !slices.Equal(names[:len(want)], want) {
		t.Errorf("Names() = %v, want the core events %v first, the core budget is deliberate", names, want)
	}
}

func TestCoreNamesNoPluginEvent(t *testing.T) {
	t.Parallel()

	for _, name := range []event.Name{"whatsapp.message.received", "import.completed"} {
		if slices.Contains(event.CoreNames(), name) {
			t.Errorf("CoreNames() holds the plugin event %q, want only events core publishes", name)
		}
	}
}

func TestDeclareMakesAPluginEventValid(t *testing.T) {
	t.Parallel()

	if err := event.Declare("alpha", "alpha.thing.happened"); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}

	if !event.Name("alpha.thing.happened").Valid() {
		t.Error("a declared event reported invalid")
	}
	if !slices.Contains(event.Names(), event.Name("alpha.thing.happened")) {
		t.Errorf("Names() = %v, want the declared event listed", event.Names())
	}
	if _, err := event.New("alpha.thing.happened", nil); err != nil {
		t.Errorf("New() error = %v, want a declared event built", err)
	}
}

func TestDeclaredHoldsOnlyForANamePluginsDeclared(t *testing.T) {
	t.Parallel()

	if err := event.Declare("echo", "echo.thing.happened"); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}

	if !event.Name("echo.thing.happened").Declared() {
		t.Error("a name a plugin declared reported undeclared")
	}
	if event.TaskCreated.Declared() {
		t.Error("a core name reported declared by a plugin")
	}
	if event.Name("echo.never.declared").Declared() {
		t.Error("a name nobody declared reported declared")
	}
}

func TestDeclareAcceptsTheSameOwnerAgain(t *testing.T) {
	t.Parallel()

	if err := event.Declare("bravo", "bravo.thing.happened"); err != nil {
		t.Fatalf("first Declare() error = %v, want nil", err)
	}

	if err := event.Declare("bravo", "bravo.thing.happened"); err != nil {
		t.Errorf("second Declare() error = %v, want the same owner accepted again", err)
	}
}

func TestDeclareRefusesANameAnotherOwnerHolds(t *testing.T) {
	t.Parallel()

	if err := event.Declare("charlie", "shared.thing.happened"); err != nil {
		t.Fatalf("first Declare() error = %v, want nil", err)
	}

	err := event.Declare("delta", "shared.thing.happened")

	if !errors.Is(err, event.ErrNameTaken) {
		t.Fatalf("Declare() error = %v, want %v", err, event.ErrNameTaken)
	}
	if !strings.Contains(err.Error(), "charlie") || !strings.Contains(err.Error(), "delta") {
		t.Errorf("Declare() error = %q, want both owners named", err)
	}
}

func TestDeclareRefusesACoreName(t *testing.T) {
	t.Parallel()

	err := event.Declare("echo", string(event.TaskCreated))

	if !errors.Is(err, event.ErrNameTaken) {
		t.Errorf("Declare() error = %v, want %v", err, event.ErrNameTaken)
	}
}

func TestDeclareRefusesAMalformedNameAndKeepsTheOthers(t *testing.T) {
	t.Parallel()

	malformedNames := []string{
		"", "foxtrot", "Foxtrot.Created", "foxtrot..created", " foxtrot.created", "foxtrot.created.", ".foxtrot",
	}
	for _, malformed := range malformedNames {
		t.Run(malformed, func(t *testing.T) {
			t.Parallel()

			err := event.Declare("foxtrot", "foxtrot.kept.event", malformed)

			if !errors.Is(err, event.ErrMalformedName) {
				t.Errorf("Declare(%q) error = %v, want %v", malformed, err, event.ErrMalformedName)
			}
			if event.Name("foxtrot.kept.event").Valid() {
				t.Error("a well formed name declared beside a malformed one became valid, want the whole declaration refused")
			}
		})
	}
}

func TestDeclareRefusesAnEmptyOwner(t *testing.T) {
	t.Parallel()

	if err := event.Declare(" ", "golf.thing.happened"); !errors.Is(err, event.ErrEmptyOwner) {
		t.Errorf("Declare() error = %v, want %v", err, event.ErrEmptyOwner)
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
