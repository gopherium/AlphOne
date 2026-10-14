// SPDX-License-Identifier: Elastic-2.0

// Package event defines the domain events AlphOne publishes to subscribers.
package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrUnknownName reports an event name AlphOne does not publish.
var ErrUnknownName = errors.New("event: unknown name")

// ErrMalformedName reports an event name that is not lowercase words joined by dots.
var ErrMalformedName = errors.New("event: malformed name")

// ErrNameTaken reports an event name core or another plugin already publishes.
var ErrNameTaken = errors.New("event: name taken")

// ErrEmptyOwner reports a declaration that names no owner.
var ErrEmptyOwner = errors.New("event: empty owner")

// Name identifies a published event and is part of the public API.
type Name string

// Event names core publishes.
const (
	TaskCreated    Name = "task.created"
	TaskCompleted  Name = "task.completed"
	ContactCreated Name = "contact.created"
)

// core lists the event names core publishes, in the order they are listed.
var core = []Name{TaskCreated, TaskCompleted, ContactCreated}

// wellFormed matches lowercase words joined by dots, at least two of them.
var wellFormed = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

// catalog holds every event name a plugin declared, with the plugin that declared it.
var catalog = struct {
	sync.RWMutex
	owners map[Name]string
}{owners: map[Name]string{}}

// Event is one thing that happened, delivered to matching subscribers.
type Event struct {
	ID         uuid.UUID
	Name       Name
	OccurredAt time.Time
	Data       map[string]any
}

// CoreNames returns the event names core publishes.
func CoreNames() []Name {
	return slices.Clone(core)
}

// Names returns every published event name, the core ones first and then the declared ones in order.
func Names() []Name {
	catalog.RLock()
	declared := slices.Sorted(maps.Keys(catalog.owners))
	catalog.RUnlock()
	return append(slices.Clone(core), declared...)
}

// Valid reports whether core or a declared plugin publishes the name.
func (n Name) Valid() bool {
	if slices.Contains(core, n) {
		return true
	}
	catalog.RLock()
	defer catalog.RUnlock()
	_, declared := catalog.owners[n]
	return declared
}

// Declare records that owner publishes the named events, refusing the whole declaration on one bad name.
func Declare(owner string, names ...string) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return ErrEmptyOwner
	}
	if err := wellFormedNames(owner, names); err != nil {
		return err
	}
	catalog.Lock()
	defer catalog.Unlock()
	if err := claimable(owner, names); err != nil {
		return err
	}
	for _, name := range names {
		catalog.owners[Name(name)] = owner
	}
	return nil
}

// wellFormedNames refuses the first name owner declared that is not lowercase words joined by dots.
func wellFormedNames(owner string, names []string) error {
	for _, name := range names {
		if !wellFormed.MatchString(name) {
			return fmt.Errorf("%w: %q declared by %s", ErrMalformedName, name, owner)
		}
	}
	return nil
}

// claimable refuses the first name owner declared that core or another plugin publishes, under the catalog lock.
func claimable(owner string, names []string) error {
	for _, name := range names {
		if slices.Contains(core, Name(name)) {
			return fmt.Errorf("%w: %q declared by %s is a core event", ErrNameTaken, name, owner)
		}
		if holder, held := catalog.owners[Name(name)]; held && holder != owner {
			return fmt.Errorf("%w: %q declared by %s is held by %s", ErrNameTaken, name, owner, holder)
		}
	}
	return nil
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
