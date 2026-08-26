// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/task"
)

var taskDay = time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)

// storeTask stores one task built from the input and returns it.
func storeTask(t *testing.T, store *postgres.TaskStore, in task.Input) task.Task {
	t.Helper()
	created := mustTask(t, in)
	if _, _, err := store.Create(t.Context(), created); err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	return created
}

// completed sets the stored status of the named task to done.
func completed(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		"UPDATE core.tasks SET status = $2 WHERE id = $1", id, task.StatusDone); err != nil {
		t.Fatalf("completing task: %v", err)
	}
}

// titlesOf returns the titles of the given tasks in order.
func titlesOf(tasks []task.Task) []string {
	titles := make([]string, len(tasks))
	for i, listed := range tasks {
		titles[i] = listed.Title
	}
	return titles
}

func TestTaskStoreListsADay(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	storeTask(t, store, task.Input{Title: "Today first", DueOn: taskDay, AssigneeID: assignee})
	storeTask(t, store, task.Input{Title: "Today second", DueOn: taskDay, AssigneeID: assignee})
	storeTask(t, store, task.Input{
		Title: "Yesterday", DueOn: taskDay.AddDate(0, 0, -1), AssigneeID: assignee,
	})
	storeTask(t, store, task.Input{
		Title: "Tomorrow", DueOn: taskDay.AddDate(0, 0, 1), AssigneeID: assignee,
	})
	storeTask(t, store, task.Input{Title: "Someone else's day", DueOn: taskDay, AssigneeID: other})

	listed, err := store.ListForDay(t.Context(), assignee, taskDay, "all", task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForDay() error = %v, want nil", err)
	}
	want := []string{"Today first", "Today second"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreFiltersADayByStatus(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	open := storeTask(t, store, task.Input{Title: "Still open", DueOn: taskDay, AssigneeID: assignee})
	done := storeTask(t, store, task.Input{Title: "Already done", DueOn: taskDay, AssigneeID: assignee})
	completed(t, pool, done.ID)

	tests := map[string]struct {
		status string
		want   []string
	}{
		"open": {status: task.StatusOpen, want: []string{open.Title}},
		"done": {status: task.StatusDone, want: []string{done.Title}},
		"all":  {status: "all", want: []string{open.Title, done.Title}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			listed, err := store.ListForDay(t.Context(), assignee, taskDay, tc.status, task.Page{Limit: 10})

			if err != nil {
				t.Fatalf("ListForDay() error = %v, want nil", err)
			}
			if got := titlesOf(listed); !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTaskStoreWalksADayByPages(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	want := []string{"First", "Second", "Third"}
	for _, title := range want {
		storeTask(t, store, task.Input{Title: title, DueOn: taskDay, AssigneeID: assignee})
	}

	var walked []string
	var sizes []int
	page := task.Page{Limit: 2}
	for {
		listed, err := store.ListForDay(t.Context(), assignee, taskDay, "all", page)
		if err != nil {
			t.Fatalf("ListForDay() error = %v, want nil", err)
		}
		sizes = append(sizes, len(listed))
		if len(listed) == 0 {
			break
		}
		walked = append(walked, titlesOf(listed)...)
		last := listed[len(listed)-1]
		page = task.Page{AfterDueOn: last.DueOn, AfterID: last.ID, Limit: 2}
	}

	if !slices.Equal(walked, want) {
		t.Errorf("walked = %v, want %v", walked, want)
	}
	if wantSizes := []int{2, 1, 0}; !slices.Equal(sizes, wantSizes) {
		t.Errorf("page sizes = %v, want %v, the limit is not being applied", sizes, wantSizes)
	}
}

func TestTaskStoreOrdersByDueDateThenID(t *testing.T) {
	t.Parallel()

	store := postgres.NewTaskStore(newTestPool(t))
	assignee := uuid.Must(uuid.NewV7())
	later := storeTask(t, store, task.Input{
		Title: "Created first, due last", DueOn: taskDay.AddDate(0, 0, 2), AssigneeID: assignee,
	})
	earlier := storeTask(t, store, task.Input{
		Title: "Created last, due first", DueOn: taskDay, AssigneeID: assignee,
	})
	if earlier.ID.String() < later.ID.String() {
		t.Fatalf("ids are not creation ordered, the ordering assertion would be meaningless")
	}

	listed, err := store.ListDueBefore(
		t.Context(), assignee, taskDay.AddDate(0, 0, 3), "all", task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListDueBefore() error = %v, want nil", err)
	}
	want := []string{earlier.Title, later.Title}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v ordered by due date rather than id", got, want)
	}
}

func TestTaskStoreWalksTheOtherModesByPages(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	owner := mustContact(t, "María Pérez")
	if err := contacts.Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	for i, title := range []string{"Oldest", "Middle", "Newest"} {
		storeTask(t, store, task.Input{
			Title:      title,
			DueOn:      taskDay.AddDate(0, 0, i-5),
			AssigneeID: assignee,
			ContactID:  owner.ID,
		})
	}

	tests := map[string]func(page task.Page) ([]task.Task, error){
		"due before": func(page task.Page) ([]task.Task, error) {
			return store.ListDueBefore(t.Context(), assignee, taskDay, "all", page)
		},
		"for contact": func(page task.Page) ([]task.Task, error) {
			return store.ListForContact(t.Context(), owner.ID, "all", page)
		},
	}

	for name, list := range tests {
		t.Run(name, func(t *testing.T) {
			first, err := list(task.Page{Limit: 2})
			if err != nil {
				t.Fatalf("first page error = %v, want nil", err)
			}
			if got := titlesOf(first); !slices.Equal(got, []string{"Oldest", "Middle"}) {
				t.Fatalf("first page = %v, want [Oldest Middle]", got)
			}

			last := first[len(first)-1]
			second, err := list(task.Page{AfterDueOn: last.DueOn, AfterID: last.ID, Limit: 2})

			if err != nil {
				t.Fatalf("second page error = %v, want nil", err)
			}
			if got := titlesOf(second); !slices.Equal(got, []string{"Newest"}) {
				t.Errorf("second page = %v, want [Newest]", got)
			}
		})
	}
}

func TestTaskStoreFiltersAContactByStatus(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	store := postgres.NewTaskStore(pool)
	owner := mustContact(t, "María Pérez")
	if err := contacts.Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	open := storeTask(t, store, task.Input{
		Title: "Still open", DueOn: taskDay, AssigneeID: uuid.Must(uuid.NewV7()), ContactID: owner.ID,
	})
	done := storeTask(t, store, task.Input{
		Title: "Already done", DueOn: taskDay, AssigneeID: uuid.Must(uuid.NewV7()), ContactID: owner.ID,
	})
	completed(t, pool, done.ID)

	tests := map[string]struct {
		status string
		want   []string
	}{
		"open": {status: task.StatusOpen, want: []string{open.Title}},
		"done": {status: task.StatusDone, want: []string{done.Title}},
		"all":  {status: "all", want: []string{open.Title, done.Title}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			listed, err := store.ListForContact(t.Context(), owner.ID, tc.status, task.Page{Limit: 10})

			if err != nil {
				t.Fatalf("ListForContact() error = %v, want nil", err)
			}
			if got := titlesOf(listed); !slices.Equal(got, tc.want) {
				t.Errorf("titles = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTaskStoreListsWhatIsDueBefore(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	other := uuid.Must(uuid.NewV7())
	storeTask(t, store, task.Input{
		Title: "Two days ago", DueOn: taskDay.AddDate(0, 0, -2), AssigneeID: assignee,
	})
	storeTask(t, store, task.Input{
		Title: "Yesterday", DueOn: taskDay.AddDate(0, 0, -1), AssigneeID: assignee,
	})
	storeTask(t, store, task.Input{Title: "Today", DueOn: taskDay, AssigneeID: assignee})
	storeTask(t, store, task.Input{
		Title: "Someone else's overdue", DueOn: taskDay.AddDate(0, 0, -1), AssigneeID: other,
	})
	closed := storeTask(t, store, task.Input{
		Title: "Overdue but done", DueOn: taskDay.AddDate(0, 0, -1), AssigneeID: assignee,
	})
	completed(t, pool, closed.ID)

	listed, err := store.ListDueBefore(t.Context(), assignee, taskDay, task.StatusOpen, task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListDueBefore() error = %v, want nil", err)
	}
	want := []string{"Two days ago", "Yesterday"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreListsAContactAcrossAssignees(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	contacts := postgres.NewContactStore(pool)
	store := postgres.NewTaskStore(pool)
	owner := mustContact(t, "María Pérez")
	if err := contacts.Create(t.Context(), owner); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	stranger := mustContact(t, "John Doe")
	if err := contacts.Create(t.Context(), stranger); err != nil {
		t.Fatalf("creating contact: %v", err)
	}
	storeTask(t, store, task.Input{
		Title: "Call her back", DueOn: taskDay, AssigneeID: uuid.Must(uuid.NewV7()), ContactID: owner.ID,
	})
	storeTask(t, store, task.Input{
		Title: "Send her the quote", DueOn: taskDay.AddDate(0, 0, 1),
		AssigneeID: uuid.Must(uuid.NewV7()), ContactID: owner.ID,
	})
	storeTask(t, store, task.Input{
		Title: "Unrelated contact", DueOn: taskDay, AssigneeID: uuid.Must(uuid.NewV7()), ContactID: stranger.ID,
	})
	storeTask(t, store, task.Input{
		Title: "Unlinked task", DueOn: taskDay, AssigneeID: uuid.Must(uuid.NewV7()),
	})

	listed, err := store.ListForContact(t.Context(), owner.ID, "all", task.Page{Limit: 10})

	if err != nil {
		t.Fatalf("ListForContact() error = %v, want nil", err)
	}
	want := []string{"Call her back", "Send her the quote"}
	if got := titlesOf(listed); !slices.Equal(got, want) {
		t.Errorf("titles = %v, want %v", got, want)
	}
}

func TestTaskStoreReportsListConnectionFailures(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewTaskStore(pool)
	assignee := uuid.Must(uuid.NewV7())
	pool.Close()

	if _, err := store.ListForDay(t.Context(), assignee, taskDay, "all", task.Page{Limit: 10}); err == nil {
		t.Error("ListForDay() on closed pool error = nil, want error")
	}
	if _, err := store.ListDueBefore(t.Context(), assignee, taskDay, "all", task.Page{Limit: 10}); err == nil {
		t.Error("ListDueBefore() on closed pool error = nil, want error")
	}
	if _, err := store.ListForContact(t.Context(), assignee, "all", task.Page{Limit: 10}); err == nil {
		t.Error("ListForContact() on closed pool error = nil, want error")
	}
}
