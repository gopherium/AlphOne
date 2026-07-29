// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
)

func mustTask(t *testing.T, in task.Input) task.Task {
	t.Helper()
	created, err := task.New(in)
	if err != nil {
		t.Fatalf("task.New() error = %v, want nil", err)
	}
	return created
}

func TestTaskStoreRoundTrip(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	store := postgres.NewTaskStore(pool)
	owner := mustContact(t, "María Pérez")
	if err := contacts.Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	created := mustTask(t, task.Input{
		Title:      "Call María about the wicker lamp",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		Priority:   1,
		AssigneeID: uuid.Must(uuid.NewV7()),
		ContactID:  owner.ID,
		Origin:     task.Origin{Source: "seed", EventID: uuid.Must(uuid.NewV7())},
	})

	if err := store.Create(t.Context(), created); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if diff := cmp.Diff(created, got, cmpopts.EquateApproxTime(time.Second)); diff != "" {
		t.Errorf("Get() mismatch (-want +got):\n%s", diff)
	}
}

func TestTaskStoreRoundTripsAnUnlinkedTask(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	created := mustTask(t, task.Input{
		Title:      "Review the quarterly numbers",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
	})

	if err := store.Create(t.Context(), created); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	got, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ContactID != uuid.Nil {
		t.Errorf("ContactID = %v, want uuid.Nil", got.ContactID)
	}
	if got.Origin != (task.Origin{}) {
		t.Errorf("Origin = %+v, want zero", got.Origin)
	}
}

func TestTaskStoreClearsTheContactLinkWhenTheContactGoes(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	store := postgres.NewTaskStore(pool)
	owner := mustContact(t, "María Pérez")
	if err := contacts.Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	created := mustTask(t, task.Input{
		Title:      "Call María about the wicker lamp",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
		ContactID:  owner.ID,
	})
	if err := store.Create(t.Context(), created); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	if _, err := pool.Exec(t.Context(), "DELETE FROM core.contacts WHERE id = $1", owner.ID); err != nil {
		t.Fatalf("deleting contact: %v", err)
	}

	got, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if got.ContactID != uuid.Nil {
		t.Errorf("ContactID = %v, want uuid.Nil after the contact went away", got.ContactID)
	}
}

func TestTaskStoreUpdatesATask(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	created := mustTask(t, task.Input{
		Title:      "Call the supplier",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
		Origin:     task.Origin{Source: "seed", EventID: uuid.Must(uuid.NewV7())},
	})
	if err := store.Create(t.Context(), created); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	title := "Call the supplier back"
	dueOn := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	status := task.StatusDone
	priority := 2
	changed, err := created.Apply(task.Changes{
		Title: &title, DueOn: &dueOn, Status: &status, Priority: &priority,
	})
	if err != nil {
		t.Fatalf("Apply() error = %v, want nil", err)
	}

	updated, err := store.Update(t.Context(), changed)

	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}
	if diff := cmp.Diff(changed, updated, cmpopts.EquateApproxTime(time.Second)); diff != "" {
		t.Errorf("Update() mismatch (-want +got):\n%s", diff)
	}
	stored, err := store.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if stored.Title != title || stored.Status != task.StatusDone || stored.Priority != priority {
		t.Errorf("stored task = %+v, want the updated fields persisted", stored)
	}
	if stored.Origin != created.Origin {
		t.Errorf("Origin = %+v, want %+v, an update must not touch it", stored.Origin, created.Origin)
	}
}

func TestTaskStoreReportsAMissingUpdate(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	absent := mustTask(t, task.Input{
		Title:      "Call the supplier",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
	})

	_, err := store.Update(t.Context(), absent)

	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Update() error = %v, want %v", err, task.ErrNotFound)
	}
}

func TestTaskStoreReportsAMissingTask(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))

	_, err := store.Get(t.Context(), uuid.Must(uuid.NewV7()))

	if !errors.Is(err, task.ErrNotFound) {
		t.Fatalf("Get() error = %v, want %v", err, task.ErrNotFound)
	}
}

func TestTaskStoreRejectsAnUnknownContact(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	created := mustTask(t, task.Input{
		Title:      "Call a contact that does not exist",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
		ContactID:  uuid.Must(uuid.NewV7()),
	})

	if err := store.Create(t.Context(), created); err == nil {
		t.Fatal("Create() error = nil, want a foreign key failure")
	}
}

func TestTaskStoreReportsConnectionFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	created := mustTask(t, task.Input{
		Title:      "Call the supplier",
		DueOn:      time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC),
		AssigneeID: uuid.Must(uuid.NewV7()),
	})
	pool.Close()

	if err := store.Create(t.Context(), created); err == nil {
		t.Error("Create() on closed pool error = nil, want error")
	}
	if _, err := store.Get(t.Context(), created.ID); err == nil || errors.Is(err, task.ErrNotFound) {
		t.Errorf("Get() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
	if _, err := store.Update(t.Context(), created); err == nil || errors.Is(err, task.ErrNotFound) {
		t.Errorf("Update() on closed pool error = %v, want a non-ErrNotFound error", err)
	}
}
