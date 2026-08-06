// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/task"
)

// errExactlyOneTaskFilter reports a tasks query without exactly one filter.
var errExactlyOneTaskFilter = errors.New("tasks: exactly one of date, dueBefore or contactId is required")

// statusAll lists tasks of every status.
const statusAll = "all"

// listStatus resolves the status argument, defaulting to open tasks.
func listStatus(status *string) (string, error) {
	switch {
	case status == nil:
		return task.StatusOpen, nil
	case *status == task.StatusOpen, *status == task.StatusDone, *status == statusAll:
		return *status, nil
	default:
		return "", task.ErrInvalidStatus
	}
}

// taskPageArgs resolves the shared task listing arguments into a page.
func taskPageArgs(status *string, first *int, after *string) (string, task.Page, int, error) {
	limit, err := pageSize(first)
	if err != nil {
		return "", task.Page{}, 0, err
	}
	st, err := listStatus(status)
	if err != nil {
		return "", task.Page{}, 0, err
	}
	page, err := cursor.DecodeTask(stringOf(after))
	if err != nil {
		return "", task.Page{}, 0, err
	}
	page.Limit = limit + 1
	return st, page, limit, nil
}

// toTask maps a domain task to its graph model.
func toTask(t task.Task) *model.Task {
	mapped := &model.Task{
		ID:         t.ID,
		AssigneeID: t.AssigneeID,
		Title:      t.Title,
		Status:     t.Status,
		Priority:   t.Priority,
		DueOn:      t.DueOn,
		CreatedAt:  t.CreatedAt.UTC(),
	}
	if t.ContactID != uuid.Nil {
		contactID := t.ContactID
		mapped.ContactID = &contactID
	}
	if t.Origin.Source != "" {
		source := t.Origin.Source
		mapped.OriginSource = &source
	}
	if t.Origin.EventID != uuid.Nil {
		eventID := t.Origin.EventID
		mapped.OriginEventID = &eventID
	}
	return mapped
}

// taskConnection assembles a task page into a connection.
func taskConnection(rows []task.Task, limit int) *model.TaskConnection {
	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	edges := make([]*model.TaskEdge, len(rows))
	for i, row := range rows {
		edges[i] = &model.TaskEdge{Node: toTask(row), Cursor: cursor.EncodeTask(row)}
	}
	pageInfo := &model.PageInfo{HasNextPage: hasNext}
	if len(edges) > 0 {
		pageInfo.StartCursor = &edges[0].Cursor
		pageInfo.EndCursor = &edges[len(edges)-1].Cursor
	}
	return &model.TaskConnection{Edges: edges, PageInfo: pageInfo}
}

// Tasks pages the acting user's tasks filtered by exactly one dimension.
func (q queryResolver) Tasks(
	ctx context.Context,
	date, dueBefore *time.Time,
	contactID *uuid.UUID,
	status *string,
	first *int,
	after *string,
) (*model.TaskConnection, error) {
	st, page, limit, err := taskPageArgs(status, first, after)
	if err != nil {
		return nil, err
	}
	rows, err := q.listTasks(ctx, date, dueBefore, contactID, st, page)
	if err != nil {
		return nil, err
	}
	return taskConnection(rows, limit), nil
}

// listTasks dispatches the listing to the store matching the present filter.
func (q queryResolver) listTasks(
	ctx context.Context,
	date, dueBefore *time.Time,
	contactID *uuid.UUID,
	status string,
	page task.Page,
) ([]task.Task, error) {
	if presentFilters(date, dueBefore, contactID) != 1 {
		return nil, errExactlyOneTaskFilter
	}
	assignee := authkit.IdentityFromContext(ctx).ID
	switch {
	case contactID != nil:
		return q.root.Tasks.ListForContact(ctx, *contactID, status, page)
	case date != nil:
		return q.root.Tasks.ListForDay(ctx, assignee, *date, status, page)
	default:
		return q.root.Tasks.ListDueBefore(ctx, assignee, *dueBefore, status, page)
	}
}

// presentFilters counts the provided filter arguments.
func presentFilters(date, dueBefore *time.Time, contactID *uuid.UUID) int {
	count := 0
	if date != nil {
		count++
	}
	if dueBefore != nil {
		count++
	}
	if contactID != nil {
		count++
	}
	return count
}

// Task returns one task by id.
func (q queryResolver) Task(ctx context.Context, id uuid.UUID) (*model.Task, error) {
	row, err := q.root.Tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toTask(row), nil
}

// taskResolver serves the Task field resolvers.
type taskResolver struct {
	root *Resolver
}

// Contact resolves the task's linked contact through the request loader.
func (t taskResolver) Contact(ctx context.Context, obj *model.Task) (*model.Contact, error) {
	if obj.ContactID == nil {
		return nil, nil
	}
	row, err := loadContact(ctx, *obj.ContactID)
	if err != nil {
		return nil, err
	}
	return toContact(row), nil
}
