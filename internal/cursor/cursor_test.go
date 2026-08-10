// SPDX-License-Identifier: Elastic-2.0

package cursor_test

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

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
	if err != nil || name != "" || id != uuid.Nil {
		t.Errorf("DecodeContact(empty) = (%q, %s, %v), want zero values and nil", name, id, err)
	}

	if _, _, err := cursor.DecodeContact("%%%"); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeContact(bad base64) error = %v, want ErrMalformed", err)
	}
	notJSON := base64.RawURLEncoding.EncodeToString([]byte("notjson"))
	if _, _, err := cursor.DecodeContact(notJSON); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeContact(bad json) error = %v, want ErrMalformed", err)
	}
}

func TestTaskCursorRoundTrip(t *testing.T) {
	t.Parallel()

	followUp := task.Task{
		ID:    uuid.MustParse("3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"),
		DueOn: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
	}

	page, err := cursor.DecodeTask(cursor.EncodeTask(followUp))

	if err != nil {
		t.Fatalf("DecodeTask() error = %v, want nil", err)
	}
	if !page.AfterDueOn.Equal(followUp.DueOn) || page.AfterID != followUp.ID {
		t.Errorf("page = %+v, want after (%s, %s)", page, followUp.DueOn, followUp.ID)
	}
}

func TestTaskCursorPinsTheWireShape(t *testing.T) {
	t.Parallel()

	followUp := task.Task{
		ID:    uuid.MustParse("3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"),
		DueOn: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC),
	}

	decoded, err := base64.RawURLEncoding.DecodeString(cursor.EncodeTask(followUp))

	if err != nil {
		t.Fatalf("decoding the cursor token: %v", err)
	}
	want := `{"due_on":"2026-08-05","id":"3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"}`
	if string(decoded) != want {
		t.Errorf("cursor payload = %s, want %s", decoded, want)
	}
}

func TestTaskCursorAbsentAndMalformed(t *testing.T) {
	t.Parallel()

	page, err := cursor.DecodeTask("")
	if err != nil || !page.AfterDueOn.IsZero() || page.AfterID != uuid.Nil {
		t.Errorf("DecodeTask(empty) = (%+v, %v), want a zero page and nil", page, err)
	}

	badDatePayload := `{"due_on":"soon","id":"3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f"}`
	badDate := base64.RawURLEncoding.EncodeToString([]byte(badDatePayload))
	if _, err := cursor.DecodeTask(badDate); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeTask(bad date) error = %v, want ErrMalformed", err)
	}

	if _, err := cursor.DecodeTask("%%%"); !errors.Is(err, cursor.ErrMalformed) {
		t.Errorf("DecodeTask(bad base64) error = %v, want ErrMalformed", err)
	}
}
