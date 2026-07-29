// SPDX-License-Identifier: Elastic-2.0

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres/db"
	"github.com/gopherium/alphone/internal/task"
)

// TaskStore persists tasks in the core schema.
type TaskStore struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

// NewTaskStore returns a [TaskStore] backed by pool.
func NewTaskStore(pool *pgxpool.Pool) *TaskStore {
	return &TaskStore{pool: pool, queries: db.New(pool)}
}

// Create stores a new task.
func (s *TaskStore) Create(ctx context.Context, t task.Task) error {
	err := s.queries.CreateTask(ctx, db.CreateTaskParams{
		ID:            t.ID,
		AssigneeID:    t.AssigneeID,
		ContactID:     optionalUUID(t.ContactID),
		Title:         t.Title,
		Status:        t.Status,
		Priority:      int16(t.Priority),
		DueOn:         t.DueOn,
		OriginSource:  optionalText(t.Origin.Source),
		OriginEventID: optionalUUID(t.Origin.EventID),
		CreatedAt:     t.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("postgres: create task: %w", err)
	}
	return nil
}

// Get returns the task with the given id, or [task.ErrNotFound] if none
// exists.
func (s *TaskStore) Get(ctx context.Context, id uuid.UUID) (task.Task, error) {
	row, err := s.queries.GetTask(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("postgres: get task: %w", err)
	}
	return taskFromRow(row), nil
}

// Update replaces the task's editable fields, returning the stored task or
// [task.ErrNotFound] when no such task exists.
func (s *TaskStore) Update(ctx context.Context, t task.Task) (task.Task, error) {
	row, err := s.queries.UpdateTask(ctx, db.UpdateTaskParams{
		ID:       t.ID,
		Title:    t.Title,
		Status:   t.Status,
		Priority: int16(t.Priority),
		DueOn:    t.DueOn,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return task.Task{}, task.ErrNotFound
	}
	if err != nil {
		return task.Task{}, fmt.Errorf("postgres: update task: %w", err)
	}
	return taskFromRow(row), nil
}

// ListForDay returns the assignee's tasks due on the given day, after the
// page cursor and narrowed to the given status.
func (s *TaskStore) ListForDay(
	ctx context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
) ([]task.Task, error) {
	rows, err := s.queries.ListTasksForDay(ctx, db.ListTasksForDayParams{
		AssigneeID: assigneeID,
		DueOn:      dueOn,
		Status:     status,
		AfterDueOn: page.AfterDueOn,
		AfterID:    page.AfterID,
		RowLimit:   int32(page.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list tasks for day: %w", err)
	}
	return tasksFromRows(rows), nil
}

// ListDueBefore returns the assignee's tasks due before the given day, after
// the page cursor and narrowed to the given status.
func (s *TaskStore) ListDueBefore(
	ctx context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
) ([]task.Task, error) {
	rows, err := s.queries.ListTasksDueBefore(ctx, db.ListTasksDueBeforeParams{
		AssigneeID: assigneeID,
		DueBefore:  dueBefore,
		Status:     status,
		AfterDueOn: page.AfterDueOn,
		AfterID:    page.AfterID,
		RowLimit:   int32(page.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list tasks due before: %w", err)
	}
	return tasksFromRows(rows), nil
}

// ListForContact returns every assignee's tasks linked to the contact, after
// the page cursor and narrowed to the given status.
func (s *TaskStore) ListForContact(
	ctx context.Context, contactID uuid.UUID, status string, page task.Page,
) ([]task.Task, error) {
	rows, err := s.queries.ListTasksForContact(ctx, db.ListTasksForContactParams{
		ContactID:  contactID,
		Status:     status,
		AfterDueOn: page.AfterDueOn,
		AfterID:    page.AfterID,
		RowLimit:   int32(page.Limit),
	})
	if err != nil {
		return nil, fmt.Errorf("postgres: list tasks for contact: %w", err)
	}
	return tasksFromRows(rows), nil
}

// tasksFromRows maps stored rows to domain tasks.
func tasksFromRows(rows []db.CoreTask) []task.Task {
	tasks := make([]task.Task, len(rows))
	for i, row := range rows {
		tasks[i] = taskFromRow(row)
	}
	return tasks
}

// taskFromRow maps a stored row to a [task.Task].
func taskFromRow(row db.CoreTask) task.Task {
	return task.Task{
		ID:         row.ID,
		AssigneeID: row.AssigneeID,
		ContactID:  storedUUID(row.ContactID),
		Title:      row.Title,
		Status:     row.Status,
		Priority:   int(row.Priority),
		DueOn:      row.DueOn,
		Origin: task.Origin{
			Source:  row.OriginSource.String,
			EventID: storedUUID(row.OriginEventID),
		},
		CreatedAt: row.CreatedAt,
	}
}

// optionalUUID converts an id to a nullable column value, treating
// [uuid.Nil] as absent.
func optionalUUID(id uuid.UUID) pgtype.UUID {
	if id == uuid.Nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: id, Valid: true}
}

// optionalText converts a string to a nullable column value, treating the
// empty string as absent.
func optionalText(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

// storedUUID converts a nullable column value to an id, reporting an absent
// value as [uuid.Nil].
func storedUUID(value pgtype.UUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return value.Bytes
}
