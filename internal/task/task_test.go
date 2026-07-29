// SPDX-License-Identifier: Elastic-2.0

package task_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/task"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

func TestNewBuildsAnOpenTask(t *testing.T) {
	t.Parallel()

	assignee := uuid.Must(uuid.NewV7())
	contactID := uuid.Must(uuid.NewV7())
	dueOn := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

	created, err := task.New(task.Input{
		Title:      "  Call María about the wicker lamp  ",
		DueOn:      dueOn,
		Priority:   1,
		AssigneeID: assignee,
		ContactID:  contactID,
	})

	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if created.Title != "Call María about the wicker lamp" {
		t.Errorf("Title = %q, want it trimmed", created.Title)
	}
	if created.Status != task.StatusOpen {
		t.Errorf("Status = %q, want %q", created.Status, task.StatusOpen)
	}
	if created.Priority != 1 {
		t.Errorf("Priority = %d, want 1", created.Priority)
	}
	if !created.DueOn.Equal(dueOn) {
		t.Errorf("DueOn = %v, want %v", created.DueOn, dueOn)
	}
	if created.AssigneeID != assignee {
		t.Errorf("AssigneeID = %v, want %v", created.AssigneeID, assignee)
	}
	if created.ContactID != contactID {
		t.Errorf("ContactID = %v, want %v", created.ContactID, contactID)
	}
	if created.Origin != (task.Origin{}) {
		t.Errorf("Origin = %+v, want zero", created.Origin)
	}
	if created.ID == uuid.Nil {
		t.Error("ID = uuid.Nil, want a generated id")
	}
	if created.CreatedAt.IsZero() || created.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want a UTC timestamp", created.CreatedAt)
	}
}

func TestNewKeepsTheOrigin(t *testing.T) {
	t.Parallel()

	origin := task.Origin{Source: "seed", EventID: uuid.Must(uuid.NewV7())}

	created, err := task.New(task.Input{
		Title:      "Review the imported numbers",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
		Origin:     origin,
	})

	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if created.Origin != origin {
		t.Errorf("Origin = %+v, want %+v", created.Origin, origin)
	}
}

func TestNewValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input   task.Input
		wantErr error
	}{
		"empty title":      {input: task.Input{Title: "", Priority: 0}, wantErr: task.ErrEmptyTitle},
		"whitespace title": {input: task.Input{Title: "   "}, wantErr: task.ErrEmptyTitle},
		"negative priority": {
			input:   task.Input{Title: "Call the supplier", Priority: -1},
			wantErr: task.ErrInvalidPriority,
		},
		"priority above nine": {
			input:   task.Input{Title: "Call the supplier", Priority: 10},
			wantErr: task.ErrInvalidPriority,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := task.New(tc.input)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewReportsIDGenerationFailure(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := task.New(task.Input{Title: "Call the supplier"})

	if !errors.Is(err, errEntropy) {
		t.Fatalf("New() error = %v, want the entropy failure in its chain", err)
	}
}
