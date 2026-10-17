// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
)

// clockAt returns the time of day written as HH:MM, failing the test when it is malformed.
func clockAt(t *testing.T, text string) task.Clock {
	t.Helper()
	clock, err := task.ParseClock(text)
	if err != nil {
		t.Fatalf("ParseClock(%q) error = %v, want nil", text, err)
	}
	return clock
}

// storeAll stores one task per input for the assignee in the order given and returns them.
func storeAll(t *testing.T, store *postgres.TaskStore, assignee uuid.UUID, inputs []task.Input) []task.Task {
	t.Helper()
	stored := make([]task.Task, len(inputs))
	for i, in := range inputs {
		in.AssigneeID = assignee
		stored[i] = storeTask(t, store, in)
	}
	return stored
}

// wantSameTask fails the test when got differs from want beyond the stored precision of the creation time.
func wantSameTask(t *testing.T, call string, want, got task.Task) {
	t.Helper()
	if diff := cmp.Diff(want, got, cmpopts.EquateApproxTime(time.Second)); diff != "" {
		t.Errorf("%s mismatch (-want +got):\n%s", call, diff)
	}
}

// wantTitles fails the test unless the walk or listing named the titles in the given order.
func wantTitles(t *testing.T, what string, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestTaskStoreRoundTripsTheTimeOfDay(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	tests := map[string]task.Clock{
		"at eleven":       clockAt(t, "11:00"),
		"at midnight":     clockAt(t, "00:00"),
		"at the last one": clockAt(t, "23:59"),
		"with no time":    {},
	}

	for name, dueTime := range tests {
		created := mustTask(t, task.Input{
			Title: "Call Maria Perez", DueOn: taskDay, DueTime: dueTime, AssigneeID: uuid.Must(uuid.NewV7()),
		})

		stored, _, err := store.Create(t.Context(), created)
		if err != nil {
			t.Fatalf("%s: Create() error = %v, want nil", name, err)
		}
		got, err := store.Get(t.Context(), created.ID)
		if err != nil {
			t.Fatalf("%s: Get() error = %v, want nil", name, err)
		}

		wantSameTask(t, name+": Create()", created, stored)
		wantSameTask(t, name+": Get()", created, got)
	}
}

func TestTaskStoreUpdatesTheTimeOfDay(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	held := storeTask(t, store, task.Input{
		Title: "Call Maria Perez", DueOn: taskDay, DueTime: clockAt(t, "11:00"), AssigneeID: uuid.Must(uuid.NewV7()),
	})
	steps := []struct {
		name    string
		changes task.Changes
		want    task.Clock
	}{
		{"a new time", task.Changes{DueTime: new(clockAt(t, "09:30"))}, clockAt(t, "09:30")},
		{"another day", task.Changes{DueOn: new(taskDay.AddDate(0, 0, 1))}, clockAt(t, "09:30")},
		{"no time", task.Changes{DueTime: &task.Clock{}}, task.Clock{}},
		{"a time again", task.Changes{DueTime: new(clockAt(t, "00:00"))}, clockAt(t, "00:00")},
	}

	for _, step := range steps {
		changed, err := held.Apply(step.changes)
		if err != nil {
			t.Fatalf("%s: Apply() error = %v, want nil", step.name, err)
		}
		updated, err := store.Update(t.Context(), changed)
		if err != nil {
			t.Fatalf("%s: Update() error = %v, want nil", step.name, err)
		}
		stored, err := store.Get(t.Context(), held.ID)
		if err != nil {
			t.Fatalf("%s: Get() error = %v, want nil", step.name, err)
		}

		wantSameTask(t, step.name+": Update()", changed, updated)
		wantSameTask(t, step.name+": Get()", changed, stored)
		if stored.DueTime != step.want {
			t.Errorf("%s: stored DueTime = %#v, want %#v", step.name, stored.DueTime, step.want)
		}
		held = stored
	}
}

func TestTaskStoreCreateKeepsTheFirstTimeOnAReplay(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	origin := task.Origin{Source: "token:n8n production", EventID: uuid.Must(uuid.NewV7())}
	assignee := uuid.Must(uuid.NewV7())
	first := storeTask(t, store, task.Input{
		Title: "Call Maria Perez", DueOn: taskDay, DueTime: clockAt(t, "11:00"), AssigneeID: assignee, Origin: origin,
	})
	replay := mustTask(t, task.Input{
		Title: "Call Maria Perez", DueOn: taskDay, DueTime: clockAt(t, "09:00"), AssigneeID: assignee, Origin: origin,
	})

	replayed, created, err := store.Create(t.Context(), replay)

	if err != nil || created {
		t.Fatalf("Create() on replay = created %v, error %v, want the stored task and no error", created, err)
	}
	wantSameTask(t, "Create() on replay", first, replayed)
}

func TestTaskStoreListsADayByPriorityThenHour(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	storeAll(t, store, assignee, []task.Input{
		{Title: "Raised with no time", DueOn: taskDay, Priority: 1},
		{Title: "Raised at eleven", DueOn: taskDay, Priority: 1, DueTime: clockAt(t, "11:00")},
		{Title: "Raised at nine", DueOn: taskDay, Priority: 1, DueTime: clockAt(t, "09:00")},
		{Title: "Plain with no time", DueOn: taskDay},
		{Title: "Plain at ten", DueOn: taskDay, DueTime: clockAt(t, "10:00")},
	})

	listed, err := store.ListForDay(t.Context(), assignee, taskDay, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForDay() error = %v, want nil", err)
	}
	want := []string{"Raised at nine", "Raised at eleven", "Raised with no time", "Plain at ten", "Plain with no time"}
	wantTitles(t, "titles", titlesOf(listed), want)
}

func TestTaskStoreListsTheBacklogByPriorityThenDayThenHour(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	early, late := taskDay.AddDate(0, 0, -3), taskDay.AddDate(0, 0, -1)
	storeAll(t, store, assignee, []task.Input{
		{Title: "Raised early at five", DueOn: early, Priority: 1, DueTime: clockAt(t, "17:00")},
		{Title: "Raised late at eight", DueOn: late, Priority: 1, DueTime: clockAt(t, "08:00")},
		{Title: "Raised early at nine", DueOn: early, Priority: 1, DueTime: clockAt(t, "09:00")},
		{Title: "Plain early with no time", DueOn: early},
		{Title: "Plain late at seven", DueOn: late, DueTime: clockAt(t, "07:00")},
		{Title: "Plain early at noon", DueOn: early, DueTime: clockAt(t, "12:00")},
	})

	listed, err := store.ListDueBefore(t.Context(), assignee, taskDay, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListDueBefore() error = %v, want nil", err)
	}
	want := []string{
		"Raised early at nine", "Raised early at five", "Raised late at eight",
		"Plain early at noon", "Plain early with no time", "Plain late at seven",
	}
	wantTitles(t, "titles", titlesOf(listed), want)
}

func TestTaskStoreListsAContactsDayByHour(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	held := mustContact(t, "Maria Perez")
	if err := postgres.NewContactStore(pool).Create(t.Context(), held); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	inputs := []task.Input{
		{Title: "Update her address", DueOn: taskDay},
		{Title: "Send her the quote", DueOn: taskDay, DueTime: clockAt(t, "11:00")},
		{Title: "Call her back", DueOn: taskDay, DueTime: clockAt(t, "09:00")},
		{Title: "Visit her tomorrow", DueOn: taskDay.AddDate(0, 0, 1), DueTime: clockAt(t, "08:00")},
	}
	for _, in := range inputs {
		in.ContactID = held.ID
		in.AssigneeID = uuid.Must(uuid.NewV7())
		storeTask(t, store, in)
	}

	listed, err := store.ListForContact(t.Context(), held.ID, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForContact() error = %v, want nil", err)
	}
	want := []string{"Call her back", "Send her the quote", "Update her address", "Visit her tomorrow"}
	wantTitles(t, "titles", titlesOf(listed), want)
}

func TestTaskStoreWalksADoneDayInCreationOrderWhateverItsHours(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	held := storeAll(t, store, assignee, []task.Input{
		{Title: "Booked at eleven", DueOn: taskDay, Priority: 1, DueTime: clockAt(t, "11:00")},
		{Title: "Booked at nine", DueOn: taskDay, DueTime: clockAt(t, "09:00")},
		{Title: "Booked at ten", DueOn: taskDay, DueTime: clockAt(t, "10:00")},
	})
	for _, done := range held {
		completed(t, pool, done.ID)
	}

	walked := walkPages(t, func(page task.Page) ([]task.Task, error) {
		return store.ListForDay(t.Context(), assignee, taskDay, task.StatusDone, page)
	}, 2)

	wantTitles(t, "walked", walked, []string{"Booked at eleven", "Booked at nine", "Booked at ten"})
}

func TestTaskStoreWalksAcrossTheTimedAndUntimedWorkOfOnePriority(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	held := mustContact(t, "Maria Perez")
	if err := postgres.NewContactStore(pool).Create(t.Context(), held); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	assignee := uuid.Must(uuid.NewV7())
	storeAll(t, store, assignee, []task.Input{
		{Title: "No time first", DueOn: taskDay, ContactID: held.ID},
		{Title: "At eleven", DueOn: taskDay, ContactID: held.ID, DueTime: clockAt(t, "11:00")},
		{Title: "No time second", DueOn: taskDay, ContactID: held.ID},
		{Title: "At nine", DueOn: taskDay, ContactID: held.ID, DueTime: clockAt(t, "09:00")},
		{Title: "Raised with no time", DueOn: taskDay, ContactID: held.ID, Priority: 1},
		{Title: "At midnight", DueOn: taskDay, ContactID: held.ID, DueTime: clockAt(t, "00:00")},
	})
	lists := map[string]rankedLister{
		"day": func(page task.Page) ([]task.Task, error) {
			return store.ListForDay(t.Context(), assignee, taskDay, task.StatusOpen, page)
		},
		"backlog": func(page task.Page) ([]task.Task, error) {
			return store.ListDueBefore(t.Context(), assignee, taskDay.AddDate(0, 0, 1), task.StatusOpen, page)
		},
		"contact": func(page task.Page) ([]task.Task, error) {
			return store.ListForContact(t.Context(), held.ID, task.StatusOpen, page)
		},
	}
	want := []string{"Raised with no time", "At midnight", "At nine", "At eleven", "No time first", "No time second"}

	for name, list := range lists {
		for _, limit := range []int{1, 2} {
			wantTitles(t, fmt.Sprintf("%s walked %d at a time", name, limit), walkPages(t, list, limit), want)
		}
	}
}

func TestTaskStoreListsTimedOpenWorkBeforeTheDoneWorkOfItsDay(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	held := storeAll(t, store, assignee, []task.Input{
		{Title: "Plain with no time", DueOn: taskDay},
		{Title: "Done at eight", DueOn: taskDay, DueTime: clockAt(t, "08:00")},
		{Title: "Plain at nine", DueOn: taskDay, DueTime: clockAt(t, "09:00")},
		{Title: "Raised with no time", DueOn: taskDay, Priority: 1},
		{Title: "Done with no time", DueOn: taskDay},
	})
	completed(t, pool, held[1].ID)
	completed(t, pool, held[4].ID)

	walked := walkPages(t, func(page task.Page) ([]task.Task, error) {
		return store.ListForDay(t.Context(), assignee, taskDay, "all", page)
	}, 2)

	want := []string{"Raised with no time", "Plain at nine", "Plain with no time", "Done at eight", "Done with no time"}
	wantTitles(t, "walked", walked, want)
}
