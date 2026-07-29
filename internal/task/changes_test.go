// SPDX-License-Identifier: Elastic-2.0

package task_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/task"
)

func openTask(t *testing.T) task.Task {
	t.Helper()
	created, err := task.New(task.Input{
		Title:      "Call the supplier",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
	})
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	return created
}

func TestApplyReplacesTheGivenFields(t *testing.T) {
	t.Parallel()

	original := openTask(t)
	title := "  Call the supplier back  "
	dueOn := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	status := task.StatusDone
	priority := 2

	updated, err := original.Apply(task.Changes{
		Title:    &title,
		DueOn:    &dueOn,
		Status:   &status,
		Priority: &priority,
	})

	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if updated.Title != "Call the supplier back" {
		t.Errorf("Title = %q, want it trimmed", updated.Title)
	}
	if !updated.DueOn.Equal(dueOn) {
		t.Errorf("DueOn = %v, want %v", updated.DueOn, dueOn)
	}
	if updated.Status != task.StatusDone {
		t.Errorf("Status = %q, want %q", updated.Status, task.StatusDone)
	}
	if updated.Priority != priority {
		t.Errorf("Priority = %d, want %d", updated.Priority, priority)
	}
	if updated.ID != original.ID || updated.AssigneeID != original.AssigneeID {
		t.Error("Apply() changed the task identity, want it preserved")
	}
	if !updated.CreatedAt.Equal(original.CreatedAt) {
		t.Error("Apply() changed the creation time, want it preserved")
	}
}

func TestApplyLeavesOmittedFieldsAlone(t *testing.T) {
	t.Parallel()

	original := openTask(t)
	status := task.StatusDone

	updated, err := original.Apply(task.Changes{Status: &status})

	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if updated.Title != original.Title {
		t.Errorf("Title = %q, want it unchanged", updated.Title)
	}
	if !updated.DueOn.Equal(original.DueOn) {
		t.Errorf("DueOn = %v, want it unchanged", updated.DueOn)
	}
	if updated.Priority != original.Priority {
		t.Errorf("Priority = %d, want it unchanged", updated.Priority)
	}
	if updated.Status != task.StatusDone {
		t.Errorf("Status = %q, want %q", updated.Status, task.StatusDone)
	}
}

func TestApplyWithoutChangesReturnsTheSameTask(t *testing.T) {
	t.Parallel()

	original := openTask(t)

	updated, err := original.Apply(task.Changes{})

	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}
	if updated != original {
		t.Errorf("Apply() = %+v, want %+v", updated, original)
	}
}

func TestApplyRejectsInvalidChanges(t *testing.T) {
	t.Parallel()

	blank := "   "
	unknown := "archived"
	filter := "all"
	tooHigh := 10
	tests := map[string]struct {
		changes task.Changes
		wantErr error
	}{
		"blank title":    {changes: task.Changes{Title: &blank}, wantErr: task.ErrEmptyTitle},
		"unknown status": {changes: task.Changes{Status: &unknown}, wantErr: task.ErrInvalidStatus},
		"list filter as status": {
			changes: task.Changes{Status: &filter},
			wantErr: task.ErrInvalidStatus,
		},
		"priority out of range": {changes: task.Changes{Priority: &tooHigh}, wantErr: task.ErrInvalidPriority},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := openTask(t).Apply(tc.changes)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Apply() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateStatusAcceptsOnlyStoredStatuses(t *testing.T) {
	t.Parallel()

	tests := map[string]error{
		task.StatusOpen: nil,
		task.StatusDone: nil,
		"all":           task.ErrInvalidStatus,
		"":              task.ErrInvalidStatus,
		"Open":          task.ErrInvalidStatus,
	}

	for status, want := range tests {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()

			if err := task.ValidateStatus(status); !errors.Is(err, want) {
				t.Fatalf("ValidateStatus(%q) error = %v, want %v", status, err, want)
			}
		})
	}
}
