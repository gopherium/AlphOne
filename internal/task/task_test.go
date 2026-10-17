// SPDX-License-Identifier: Elastic-2.0

package task_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/task"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

// Read reports the entropy failure and reads nothing.
func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

func TestRankPutsHigherPriorityOpenWorkFirst(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		held task.Task
		want int
	}{
		"open at normal priority": {task.Task{Status: task.StatusOpen}, 0},
		"open at priority 1":      {task.Task{Status: task.StatusOpen, Priority: 1}, -1},
		"open at priority 9":      {task.Task{Status: task.StatusOpen, Priority: 9}, -9},
		"done at priority 5":      {task.Task{Status: task.StatusDone, Priority: 5}, 0},
	}
	for name, tc := range tests {
		if got := tc.held.Rank(); got != tc.want {
			t.Errorf("%s: Rank() = %d, want %d", name, got, tc.want)
		}
	}
}

func TestSortClockKeepsTheTimeOfOpenTimedWorkOnly(t *testing.T) {
	t.Parallel()

	nine := task.Clock{Minutes: 540, Set: true}
	tests := map[string]struct {
		held task.Task
		want task.Clock
	}{
		"open at nine":      {task.Task{Status: task.StatusOpen, DueTime: nine}, nine},
		"open with no time": {task.Task{Status: task.StatusOpen}, task.Clock{}},
		"done at nine":      {task.Task{Status: task.StatusDone, DueTime: nine}, task.Clock{}},
		"done with no time": {task.Task{Status: task.StatusDone}, task.Clock{}},
	}
	for name, tc := range tests {
		if got := tc.held.SortClock(); got != tc.want {
			t.Errorf("%s: SortClock() = %#v, want %#v", name, got, tc.want)
		}
	}
}

func TestPageAfterStartsAfterTheLastTask(t *testing.T) {
	t.Parallel()

	id := uuid.MustParse("3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f")
	day := time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC)
	nine := task.Clock{Minutes: 540, Set: true}
	tests := map[string]struct {
		last task.Task
		want task.Page
	}{
		"open raised at nine": {
			last: task.Task{ID: id, Status: task.StatusOpen, Priority: 1, DueOn: day, DueTime: nine},
			want: task.Page{AfterRank: -1, AfterDueOn: day, AfterClock: nine, AfterID: id, Limit: 7},
		},
		"open with no time": {
			last: task.Task{ID: id, Status: task.StatusOpen, DueOn: day},
			want: task.Page{AfterDueOn: day, AfterID: id, Limit: 7},
		},
		"done at nine": {
			last: task.Task{ID: id, Status: task.StatusDone, Priority: 1, DueOn: day, DueTime: nine},
			want: task.Page{AfterDueOn: day, AfterID: id, Limit: 7},
		},
	}
	for name, tc := range tests {
		if got := task.PageAfter(tc.last, 7); got != tc.want {
			t.Errorf("%s: PageAfter() = %#v, want %#v", name, got, tc.want)
		}
	}
}

func TestAPageStartsBelowEveryRankOnlyWithoutAPosition(t *testing.T) {
	t.Parallel()

	highest := task.Task{Status: task.StatusOpen, Priority: 9}
	if start := (task.Page{}).StartRank(); start >= highest.Rank() {
		t.Errorf("first page StartRank() = %d, want below %d", start, highest.Rank())
	}
	after := task.Page{AfterRank: -3, AfterID: uuid.MustParse("3b1f4d5e-6a7b-4c8d-9e0f-1a2b3c4d5e6f")}
	if start := after.StartRank(); start != -3 {
		t.Errorf("later page StartRank() = %d, want -3", start)
	}
}

func TestNewBuildsAnOpenTask(t *testing.T) {
	t.Parallel()

	assignee := uuid.Must(uuid.NewV7())
	contactID := uuid.Must(uuid.NewV7())
	dueOn := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	eleven := task.Clock{Minutes: 660, Set: true}

	created, err := task.New(task.Input{
		Title:      "  Call María about the wicker lamp  ",
		DueOn:      dueOn,
		DueTime:    eleven,
		Priority:   1,
		AssigneeID: assignee,
		ContactID:  contactID,
	})

	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	want := task.Task{
		AssigneeID: assignee,
		ContactID:  contactID,
		Title:      "Call María about the wicker lamp",
		Status:     task.StatusOpen,
		Priority:   1,
		DueOn:      dueOn,
		DueTime:    eleven,
	}
	if diff := cmp.Diff(want, created, cmpopts.IgnoreFields(task.Task{}, "ID", "CreatedAt")); diff != "" {
		t.Errorf("New() mismatch, want the title trimmed and every other value kept (-want +got):\n%s", diff)
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
		"origin event without a source": {
			input: task.Input{
				Title:  "Call the supplier",
				Origin: task.Origin{EventID: uuid.Must(uuid.NewV7())},
			},
			wantErr: task.ErrUnattributedOrigin,
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

func TestEventDataCarriesTheTaskFields(t *testing.T) {
	t.Parallel()

	id := uuid.Must(uuid.NewV7())
	day := time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC)
	eleven := task.Clock{Minutes: 11 * 60, Set: true}

	tests := map[string]struct {
		status  string
		dueTime task.Clock
		want    any
	}{
		"open with no time": {status: task.StatusOpen, want: nil},
		"open at eleven":    {status: task.StatusOpen, dueTime: eleven, want: "11:00"},
		"done at eleven":    {status: task.StatusDone, dueTime: eleven, want: "11:00"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fields := task.EventData(task.Task{
				ID: id, Title: "Call the supplier", Status: tc.status, DueOn: day, DueTime: tc.dueTime, Priority: 3,
			})

			want := map[string]any{
				"id":       id.String(),
				"title":    "Call the supplier",
				"status":   tc.status,
				"due_on":   "2026-08-06",
				"due_time": tc.want,
				"priority": 3,
			}
			if !reflect.DeepEqual(fields, want) {
				t.Errorf("EventData() = %#v, want %#v", fields, want)
			}
		})
	}
}
