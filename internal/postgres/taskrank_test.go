// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
)

// rankedLister reads one page of a task listing.
type rankedLister func(page task.Page) ([]task.Task, error)

// walkRanked reads every page of a listing two tasks at a time and returns the titles in order.
func walkRanked(t *testing.T, list rankedLister) []string {
	t.Helper()
	var walked []string
	page := task.Page{Limit: 2}
	for range 10 {
		listed, err := list(page)
		if err != nil {
			t.Fatalf("listing a page: %v", err)
		}
		if len(listed) == 0 {
			return walked
		}
		walked = append(walked, titlesOf(listed)...)
		last := listed[len(listed)-1]
		page = task.Page{AfterRank: last.Rank(), AfterDueOn: last.DueOn, AfterID: last.ID, Limit: 2}
	}
	t.Fatalf("the walk never ended, walked %v", walked)
	return nil
}

func TestTaskStoreListsHigherPriorityFirstInADay(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	storeTask(t, store, task.Input{Title: "Send the quote", DueOn: taskDay, AssigneeID: assignee})
	storeTask(t, store, task.Input{Title: "Renew the license", DueOn: taskDay, Priority: 1, AssigneeID: assignee})
	storeTask(t, store, task.Input{Title: "Restore the backup", DueOn: taskDay, Priority: 9, AssigneeID: assignee})

	listed, err := store.ListForDay(t.Context(), assignee, taskDay, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForDay() error = %v, want nil", err)
	}
	want := []string{"Restore the backup", "Renew the license", "Send the quote"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreListsTheBacklogByPriorityAcrossDays(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	storeTask(t, store, task.Input{Title: "Chase the invoice", DueOn: taskDay.AddDate(0, 0, -10), AssigneeID: assignee})
	storeTask(t, store, task.Input{
		Title: "Renew the contract", DueOn: taskDay.AddDate(0, 0, -1), Priority: 1, AssigneeID: assignee,
	})
	storeTask(t, store, task.Input{Title: "File the report", DueOn: taskDay.AddDate(0, 0, -8), AssigneeID: assignee})

	listed, err := store.ListDueBefore(t.Context(), assignee, taskDay, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListDueBefore() error = %v, want nil", err)
	}
	want := []string{"Renew the contract", "Chase the invoice", "File the report"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreKeepsDoneWorkInDueAndCreationOrder(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	plain := storeTask(t, store, task.Input{Title: "Book the courier", DueOn: taskDay, AssigneeID: assignee})
	raised := storeTask(t, store, task.Input{Title: "File the receipt", DueOn: taskDay, Priority: 1, AssigneeID: assignee})
	completed(t, pool, plain.ID)
	completed(t, pool, raised.ID)

	listed, err := store.ListForDay(t.Context(), assignee, taskDay, task.StatusDone, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForDay() error = %v, want nil", err)
	}
	want := []string{"Book the courier", "File the receipt"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreListsAContactsRaisedWorkFirst(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	held := mustContact(t, "Maria Perez")
	if err := postgres.NewContactStore(pool).Create(t.Context(), held); err != nil {
		t.Fatalf("storing the contact: %v", err)
	}
	assignee := uuid.Must(uuid.NewV7())
	storeTask(t, store, task.Input{
		Title: "Send Maria Perez the quote", DueOn: taskDay, AssigneeID: assignee, ContactID: held.ID,
	})
	storeTask(t, store, task.Input{
		Title: "Call Maria Perez back", DueOn: taskDay.AddDate(0, 0, 1), Priority: 1,
		AssigneeID: assignee, ContactID: held.ID,
	})

	listed, err := store.ListForContact(t.Context(), held.ID, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForContact() error = %v, want nil", err)
	}
	want := []string{"Call Maria Perez back", "Send Maria Perez the quote"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreWalksRankedPagesWithoutLosingATask(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	held := []task.Input{
		{Title: "Plain first", DueOn: taskDay},
		{Title: "Raised once", DueOn: taskDay, Priority: 1},
		{Title: "Plain overdue", DueOn: taskDay.AddDate(0, 0, -3)},
		{Title: "Raised most", DueOn: taskDay, Priority: 9},
		{Title: "Raised overdue", DueOn: taskDay.AddDate(0, 0, -2), Priority: 1},
		{Title: "Raised again", DueOn: taskDay, Priority: 1},
	}
	for _, in := range held {
		in.AssigneeID = assignee
		storeTask(t, store, in)
	}

	day := walkRanked(t, func(page task.Page) ([]task.Task, error) {
		return store.ListForDay(t.Context(), assignee, taskDay, task.StatusOpen, page)
	})
	backlog := walkRanked(t, func(page task.Page) ([]task.Task, error) {
		return store.ListDueBefore(t.Context(), assignee, taskDay.AddDate(0, 0, 1), task.StatusOpen, page)
	})

	if want := []string{"Raised most", "Raised once", "Raised again", "Plain first"}; !slices.Equal(day, want) {
		t.Errorf("day walked = %v, want %v", day, want)
	}
	wantBacklog := []string{"Raised most", "Raised overdue", "Raised once", "Raised again", "Plain overdue", "Plain first"}
	if !slices.Equal(backlog, wantBacklog) {
		t.Errorf("backlog walked = %v, want %v", backlog, wantBacklog)
	}
}
