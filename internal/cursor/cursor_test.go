// SPDX-License-Identifier: Elastic-2.0

package cursor_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/task"
)

func TestContactCursorRoundTrip(t *testing.T) {
	t.Parallel()

	maria := contact.Contact{ID: uuid.MustParse("6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f"), Name: "Maria Perez"}

	encoded := cursor.EncodeContact(maria)
	name, id, err := cursor.DecodeContact(encoded)

	if err != nil {
		t.Fatalf("DecodeContact() error = %v, want nil", err)
	}
	if name != maria.Name || id != maria.ID {
		t.Errorf("decoded = (%q, %s), want (%q, %s)", name, id, maria.Name, maria.ID)
	}
}

func TestContactCursorPinsTheWireShape(t *testing.T) {
	t.Parallel()

	maria := contact.Contact{ID: uuid.MustParse("6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f"), Name: "Maria Perez"}

	decoded, err := base64.RawURLEncoding.DecodeString(cursor.EncodeContact(maria))

	if err != nil {
		t.Fatalf("decoding the cursor token: %v", err)
	}
	want := `{"name":"Maria Perez","id":"6f1c2e6a-8f2b-4c1d-9e3f-5a7b9c1d2e3f"}`
	if string(decoded) != want {
		t.Errorf("cursor payload = %s, want %s", decoded, want)
	}
}

func TestContactCursorAbsentAndMalformed(t *testing.T) {
	t.Parallel()

	name, id, err := cursor.DecodeContact("")
	if err != nil {
		t.Errorf("DecodeContact(empty) error = %v, want nil", err)
	}
	if name != "" || id != uuid.Nil {
		t.Errorf("DecodeContact(empty) = (%q, %s), want zero values", name, id)
	}

	if _, _, err := cursor.DecodeContact("%%%"); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeContact(bad base64) error = %v, want ErrMalformed", err)
	}
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("notjson"))
	if _, _, err := cursor.DecodeContact(notJSON); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeContact(bad json) error = %v, want ErrMalformed", err)
	}
}

// heldID is the id of every task the task cursor tests encode.
const heldID = "3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"

// heldTasks returns one task of each sort clock shape, keyed by what it holds.
func heldTasks() map[string]task.Task {
	followUp := task.Task{
		ID:       uuid.MustParse(heldID),
		DueOn:    time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
		Status:   task.StatusOpen,
		Priority: 1,
	}
	midnight := followUp
	midnight.DueTime = task.Clock{Set: true}
	timed := followUp
	timed.DueTime = task.Clock{Minutes: 570, Set: true}
	done := timed
	done.Status = task.StatusDone
	return map[string]task.Task{
		"open with no time":      followUp,
		"open at midnight":       midnight,
		"open at half past nine": timed,
		"done at half past nine": done,
	}
}

// encodedPayload decodes the token, failing the test when it is not base64url.
func encodedPayload(t *testing.T, token string) string {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding the cursor token: %v", err)
	}
	return string(decoded)
}

// tokenOf encodes a cursor payload as base64url.
func tokenOf(payload string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func TestTaskCursorRoundTrip(t *testing.T) {
	t.Parallel()

	for name, held := range heldTasks() {
		page, err := cursor.DecodeTask(cursor.EncodeTask(held))

		if err != nil {
			t.Fatalf("%s: DecodeTask() error = %v, want nil", name, err)
		}
		if diff := cmp.Diff(task.PageAfter(held, 0), page); diff != "" {
			t.Errorf("%s: page mismatch, want the page after the task (-want +got):\n%s", name, diff)
		}
	}
}

func TestTaskCursorPinsTheWireShape(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"open with no time":      `{"rank":-1,"due_on":"2026-08-05","clock":"24:00","id":"` + heldID + `"}`,
		"open at midnight":       `{"rank":-1,"due_on":"2026-08-05","clock":"00:00","id":"` + heldID + `"}`,
		"open at half past nine": `{"rank":-1,"due_on":"2026-08-05","clock":"09:30","id":"` + heldID + `"}`,
		"done at half past nine": `{"rank":0,"due_on":"2026-08-05","clock":"24:00","id":"` + heldID + `"}`,
	}

	for name, held := range heldTasks() {
		if got := encodedPayload(t, cursor.EncodeTask(held)); got != want[name] {
			t.Errorf("%s: cursor payload = %s, want %s", name, got, want[name])
		}
	}
}

func TestTaskCursorWithoutAClockStartsAfterTheUntimedTasks(t *testing.T) {
	t.Parallel()

	written := tokenOf(`{"rank":-1,"due_on":"2026-08-05","id":"` + heldID + `"}`)

	page, err := cursor.DecodeTask(written)

	if err != nil {
		t.Fatalf("DecodeTask() error = %v, want nil", err)
	}
	if diff := cmp.Diff(task.PageAfter(heldTasks()["open with no time"], 0), page); diff != "" {
		t.Errorf("page mismatch, want the page after an untimed task (-want +got):\n%s", diff)
	}
}

func TestTaskCursorRefusesAMalformedClock(t *testing.T) {
	t.Parallel()

	for _, clock := range []string{`""`, `"9:00"`, `"24:01"`, `"25:00"`, `"11:60"`, `"09:00:30"`, `"noon"`, `930`} {
		written := tokenOf(`{"rank":0,"due_on":"2026-08-05","clock":` + clock + `,"id":"` + heldID + `"}`)

		if _, err := cursor.DecodeTask(written); !errors.Is(err, cursor.ErrMalformed) {
			t.Errorf("DecodeTask(clock %s) error = %v, want ErrMalformed", clock, err)
		}
	}
}

func TestTaskCursorAbsentAndMalformed(t *testing.T) {
	t.Parallel()

	page, err := cursor.DecodeTask("")
	if err != nil || page != (task.Page{}) {
		t.Errorf("DecodeTask(empty) = (%#v, %v), want a zero page and nil", page, err)
	}

	ranklessPayload := `{"due_on":"2026-08-05","id":"3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"}`
	rankless := base64.RawURLEncoding.EncodeToString([]byte(ranklessPayload))
	if _, err := cursor.DecodeTask(rankless); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeTask(no rank) error = %v, want ErrMalformed", err)
	}

	badDatePayload := `{"rank":0,"due_on":"soon","id":"3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"}`
	badDate := base64.RawURLEncoding.EncodeToString([]byte(badDatePayload))
	if _, err := cursor.DecodeTask(badDate); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeTask(bad date) error = %v, want ErrMalformed", err)
	}

	if _, err := cursor.DecodeTask("%%%"); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeTask(bad base64) error = %v, want ErrMalformed", err)
	}
}
