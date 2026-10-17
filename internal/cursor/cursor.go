// SPDX-License-Identifier: Elastic-2.0

// Package cursor encodes the opaque keyset pagination tokens of the list APIs.
package cursor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/task"
)

// ErrMalformed reports a cursor token that cannot be decoded.
var ErrMalformed = errors.New("cursor: malformed")

// dueDateLayout is the wire format of a task due date inside a cursor.
const dueDateLayout = "2006-01-02"

type contactCursor struct {
	Name string    `json:"name"`
	ID   uuid.UUID `json:"id"`
}

// EncodeContact renders the position after c as an opaque cursor.
func EncodeContact(c contact.Contact) string {
	encoded, _ := json.Marshal(contactCursor{Name: c.Name, ID: c.ID})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// DecodeContact parses raw into a contact position, zero values for an absent cursor.
func DecodeContact(raw string) (string, uuid.UUID, error) {
	if raw == "" {
		return "", uuid.Nil, nil
	}
	var decoded contactCursor
	if err := decodePayload(raw, &decoded); err != nil {
		return "", uuid.Nil, err
	}
	return decoded.Name, decoded.ID, nil
}

// errRankless reports a task cursor written before tasks were listed by rank.
var errRankless = errors.New("the cursor names no rank")

// endOfDay is the wire form of the sort clock of a position that holds no time.
const endOfDay = "24:00"

type taskCursor struct {
	Rank  *int      `json:"rank"`
	DueOn string    `json:"due_on"`
	Clock *string   `json:"clock"`
	ID    uuid.UUID `json:"id"`
}

// EncodeTask renders the position after t as an opaque cursor.
func EncodeTask(t task.Task) string {
	after := task.PageAfter(t, 0)
	clock := clockText(after.AfterClock)
	encoded, _ := json.Marshal(taskCursor{
		Rank:  &after.AfterRank,
		DueOn: after.AfterDueOn.Format(dueDateLayout),
		Clock: &clock,
		ID:    after.AfterID,
	})
	return base64.RawURLEncoding.EncodeToString(encoded)
}

// DecodeTask parses raw into a task page position, a zero page for an absent cursor.
func DecodeTask(raw string) (task.Page, error) {
	if raw == "" {
		return task.Page{}, nil
	}
	var decoded taskCursor
	if err := decodePayload(raw, &decoded); err != nil {
		return task.Page{}, err
	}
	if decoded.Rank == nil {
		return task.Page{}, fmt.Errorf("%w: %v", ErrMalformed, errRankless)
	}
	dueOn, err := time.Parse(dueDateLayout, decoded.DueOn)
	if err != nil {
		return task.Page{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	clock, err := readClock(decoded.Clock)
	if err != nil {
		return task.Page{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return task.Page{AfterRank: *decoded.Rank, AfterDueOn: dueOn, AfterClock: clock, AfterID: decoded.ID}, nil
}

// clockText writes a sort clock as HH:MM, 24:00 for no time.
func clockText(clock task.Clock) string {
	if !clock.Set {
		return endOfDay
	}
	return clock.String()
}

// readClock reads the sort clock a cursor names, no time for an absent clock or 24:00.
func readClock(text *string) (task.Clock, error) {
	if text == nil || *text == endOfDay {
		return task.Clock{}, nil
	}
	return task.ParseClock(*text)
}

// decodePayload parses a base64url JSON cursor token into target.
func decodePayload(raw string, target any) error {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if err := json.Unmarshal(decoded, target); err != nil {
		return fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return nil
}
