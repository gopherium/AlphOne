// SPDX-License-Identifier: Elastic-2.0

// Package event defines the domain events AlphOne publishes to subscribers.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
)

// ErrUnknownName reports an event name AlphOne does not publish.
var ErrUnknownName = errors.New("event: unknown name")

// Name identifies a published event and is part of the public API.
type Name string

// Published event names.
const (
	TaskCreated             Name = "task.created"
	TaskCompleted           Name = "task.completed"
	ContactCreated          Name = "contact.created"
	WhatsAppMessageReceived Name = "whatsapp.message.received"
	ImportCompleted         Name = "import.completed"
)

// published lists the event names AlphOne publishes.
var published = []Name{
	TaskCreated, TaskCompleted, ContactCreated, WhatsAppMessageReceived, ImportCompleted,
}

// Event is one thing that happened, delivered to matching subscribers.
type Event struct {
	ID         uuid.UUID
	Name       Name
	OccurredAt time.Time
	Data       map[string]any
}

// Names returns every event name AlphOne publishes.
func Names() []Name {
	return slices.Clone(published)
}

// Valid reports whether AlphOne publishes the name.
func (n Name) Valid() bool {
	return slices.Contains(published, n)
}

// New returns an event named name carrying data.
func New(name Name, data map[string]any) (Event, error) {
	if !name.Valid() {
		return Event{}, fmt.Errorf("%w: %q", ErrUnknownName, name)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Event{}, fmt.Errorf("event: generate id: %w", err)
	}
	return Event{ID: id, Name: name, OccurredAt: time.Now().UTC(), Data: data}, nil
}

// Payload returns the JSON body delivered to subscribers.
func (e Event) Payload() ([]byte, error) {
	body, err := json.Marshal(struct {
		ID         uuid.UUID      `json:"id"`
		Event      Name           `json:"event"`
		OccurredAt time.Time      `json:"occurred_at"`
		Data       map[string]any `json:"data"`
	}{ID: e.ID, Event: e.Name, OccurredAt: e.OccurredAt, Data: e.Data})
	if err != nil {
		return nil, fmt.Errorf("event: encode payload: %w", err)
	}
	return body, nil
}
