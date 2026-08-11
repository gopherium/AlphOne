// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// taskPageDefault is how many tasks a call reads when none is asked for.
const taskPageDefault = 50

// taskPageMax bounds how many tasks one call may read.
const taskPageMax = 100

// tasksDocument reads one page of the caller's tasks with their contacts.
const tasksDocument = `query($date: Date, $dueBefore: Date, $status: String, $first: Int!) {
	tasks(date: $date, dueBefore: $dueBefore, status: $status, first: $first) {
		edges { node { id title dueOn status priority assigneeId contactId contact { id name } } }
	}
}`

// taskNode is one task as the graph answers it.
type taskNode struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	DueOn      string `json:"dueOn"`
	Status     string `json:"status"`
	Priority   int    `json:"priority"`
	AssigneeID string `json:"assigneeId"`
	ContactID  string `json:"contactId"`
	Contact    *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"contact"`
}

// tasksData is what the task document answers with.
type tasksData struct {
	Tasks struct {
		Edges []struct {
			Node taskNode `json:"node"`
		} `json:"edges"`
	} `json:"tasks"`
}

// taskVariables resolves the arguments one task call sends.
func taskVariables(in TasksInput) map[string]any {
	variables := map[string]any{
		"status": firstNonEmpty(in.Status, "open"),
		"first":  boundedPage(in.Limit, taskPageDefault, taskPageMax),
	}
	switch {
	case in.DueBefore != "" && in.Date == "":
		variables["dueBefore"] = in.DueBefore
	case in.DueBefore != "":
		variables["date"] = in.Date
		variables["dueBefore"] = in.DueBefore
	default:
		variables["date"] = firstNonEmpty(in.Date, time.Now().UTC().Format(time.DateOnly))
	}
	return variables
}

// firstNonEmpty returns value when it is set, otherwise fallback.
func firstNonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// tasks lists the caller's tasks for one day or the backlog.
func (t *tools) tasks(ctx context.Context, in TasksInput) (*mcp.CallToolResult, TasksOutput, error) {
	var data tasksData
	if err := t.execute(ctx, tasksDocument, taskVariables(in), &data); err != nil {
		return nil, TasksOutput{}, err
	}
	out := TasksOutput{Tasks: make([]TaskItem, 0, len(data.Tasks.Edges))}
	for _, edge := range data.Tasks.Edges {
		out.Tasks = append(out.Tasks, toTaskItem(edge.Node))
	}
	return nil, out, nil
}

// toTaskItem maps one answered task onto the item an agent reads.
func toTaskItem(node taskNode) TaskItem {
	item := TaskItem{
		ID:         node.ID,
		Title:      node.Title,
		DueOn:      node.DueOn,
		Status:     node.Status,
		Priority:   node.Priority,
		AssigneeID: node.AssigneeID,
		ContactID:  node.ContactID,
	}
	if node.Contact != nil {
		item.ContactName = node.Contact.Name
	}
	return item
}
