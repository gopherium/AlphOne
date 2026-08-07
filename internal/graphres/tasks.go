// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/credential"
	"github.com/gopherium/alphone/internal/cursor"
	"github.com/gopherium/alphone/internal/event"
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
func (q QueryResolvers) Tasks(
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
func (q QueryResolvers) listTasks(
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
func (q QueryResolvers) Task(ctx context.Context, id uuid.UUID) (*model.Task, error) {
	row, err := q.root.Tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return toTask(row), nil
}

// TaskResolvers serves the Task field resolvers.
type TaskResolvers struct {
	root *Resolver
}

// Contact resolves the task's linked contact through the request loader.
func (t TaskResolvers) Contact(ctx context.Context, obj *model.Task) (*model.Contact, error) {
	if obj.ContactID == nil {
		return nil, nil
	}
	row, err := t.root.loadContact(ctx, *obj.ContactID)
	if err != nil {
		return nil, err
	}
	return toContact(row), nil
}

// intOf dereferences an optional int argument.
func intOf(raw *int) int {
	if raw == nil {
		return 0
	}
	return *raw
}

// uuidOf dereferences an optional id argument.
func uuidOf(raw *uuid.UUID) uuid.UUID {
	if raw == nil {
		return uuid.Nil
	}
	return *raw
}

// graphTaskInput builds the domain input a create mutation stands for.
func graphTaskInput(ctx context.Context, input model.CreateTaskInput) task.Input {
	return task.Input{
		Title:      input.Title,
		DueOn:      input.DueOn,
		Priority:   intOf(input.Priority),
		AssigneeID: authkit.IdentityFromContext(ctx).ID,
		ContactID:  uuidOf(input.ContactID),
		Origin:     task.Origin{Source: credential.Origin(ctx), EventID: uuidOf(input.OriginEventID)},
	}
}

// CreateTask stores a task, replaying idempotent origin creates.
func (m MutationResolvers) CreateTask(
	ctx context.Context, input model.CreateTaskInput,
) (*model.CreateTaskPayload, error) {
	created, err := task.New(graphTaskInput(ctx, input))
	if err != nil {
		return nil, err
	}
	stored, isNew, err := m.root.Tasks.Create(ctx, created)
	if err != nil {
		return nil, err
	}
	if isNew {
		m.root.publish(ctx, event.TaskCreated, task.EventData(stored))
	}
	return &model.CreateTaskPayload{Task: toTask(stored), Replay: !isNew}, nil
}

// UpdateTask applies a partial update to a task.
func (m MutationResolvers) UpdateTask(
	ctx context.Context, id uuid.UUID, input model.UpdateTaskInput,
) (*model.Task, error) {
	stored, err := m.root.Tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	changes := task.Changes{Title: input.Title, DueOn: input.DueOn, Status: input.Status, Priority: input.Priority}
	updated, err := m.patchTask(ctx, stored, changes)
	if err != nil {
		return nil, err
	}
	return toTask(updated), nil
}

// completesTask reports whether the update closed an open task.
func completesTask(stored, updated task.Task) bool {
	return stored.Status != task.StatusDone && updated.Status == task.StatusDone
}

// patchTask applies changes to stored, publishing completion when the update closes the task.
func (m MutationResolvers) patchTask(ctx context.Context, stored task.Task, changes task.Changes) (task.Task, error) {
	if changes == (task.Changes{}) {
		return stored, nil
	}
	changed, err := stored.Apply(changes)
	if err != nil {
		return task.Task{}, err
	}
	updated, err := m.root.Tasks.Update(ctx, changed)
	if err != nil {
		return task.Task{}, err
	}
	if completesTask(stored, updated) {
		m.root.publish(ctx, event.TaskCompleted, task.EventData(updated))
	}
	return updated, nil
}
