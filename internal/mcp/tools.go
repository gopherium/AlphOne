// SPDX-License-Identifier: Elastic-2.0

package mcp

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// tools holds what every tool needs to answer a call.
type tools struct {
	graph http.Handler
}

// WorkloadInput carries the arguments of workload_summary.
type WorkloadInput struct{}

// WorkloadOutput reports how much work the caller holds.
type WorkloadOutput struct {
	DueToday int  `json:"due_today" jsonschema:"how many open tasks are due today"`
	Overdue  int  `json:"overdue" jsonschema:"how many open tasks were due before today"`
	Capped   bool `json:"capped" jsonschema:"true when a count reached the page bound and is a lower bound"`
}

// TasksInput carries the arguments of list_my_tasks.
type TasksInput struct {
	Date      string `json:"date,omitempty" jsonschema:"the day to list as YYYY-MM-DD, defaults to today"`
	DueBefore string `json:"due_before,omitempty" jsonschema:"list the backlog due before this day as YYYY-MM-DD"`
	Status    string `json:"status,omitempty" jsonschema:"open, done or all, defaults to open"`
	Limit     int    `json:"limit,omitempty" jsonschema:"how many tasks to read, 1 to 100, defaults to 50"`
}

// TaskItem is one task as an agent reads it.
type TaskItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	DueOn       string `json:"due_on"`
	Status      string `json:"status"`
	Priority    int    `json:"priority"`
	ContactID   string `json:"contact_id,omitempty"`
	ContactName string `json:"contact_name,omitempty"`
}

// TasksOutput lists the tasks a call answered with.
type TasksOutput struct {
	Tasks []TaskItem `json:"tasks"`
}

// ContactsInput carries the arguments of find_contacts.
type ContactsInput struct {
	Query string `json:"query,omitempty" jsonschema:"a name or a phone number, blank lists everyone"`
	Limit int    `json:"limit,omitempty" jsonschema:"how many contacts to read, 1 to 50, defaults to 20"`
}

// Identity is one channel address a contact is reachable on.
type Identity struct {
	Channel     string `json:"channel"`
	Identifier  string `json:"identifier"`
	DisplayName string `json:"display_name,omitempty"`
}

// ContactItem is one contact as an agent reads it.
type ContactItem struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Identities   []Identity `json:"identities"`
	HasOpenTasks bool       `json:"has_open_tasks"`
}

// ContactsOutput lists the contacts a search answered with.
type ContactsOutput struct {
	Contacts []ContactItem `json:"contacts"`
}

// ContactInput carries the arguments of get_contact.
type ContactInput struct {
	ContactID string `json:"contact_id" jsonschema:"the contact uuid, taken from find_contacts, never guessed"`
}

// ContactOutput reports one contact in full.
type ContactOutput struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  string     `json:"created_at"`
	Identities []Identity `json:"identities"`
	OpenTasks  []TaskItem `json:"open_tasks"`
}

// register declares every tool AlphOne serves, in a fixed order.
func register(server *mcp.Server, run *tools) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "workload_summary",
		Description: workloadDescription,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in WorkloadInput) (*mcp.CallToolResult, WorkloadOutput, error) {
		return run.workload(ctx, in)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "list_my_tasks",
		Description: tasksDescription,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in TasksInput) (*mcp.CallToolResult, TasksOutput, error) {
		return run.tasks(ctx, in)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "find_contacts",
		Description: contactsDescription,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContactsInput) (*mcp.CallToolResult, ContactsOutput, error) {
		return run.contacts(ctx, in)
	})
	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_contact",
		Description: contactDescription,
		Annotations: readOnly(),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ContactInput) (*mcp.CallToolResult, ContactOutput, error) {
		return run.contact(ctx, in)
	})
}
