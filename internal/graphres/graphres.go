// SPDX-License-Identifier: Elastic-2.0

// Package graphres resolves the GraphQL schema over the core services.
package graphres

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/task"
)

// ContactStore provides the contact reads the graph resolves from.
type ContactStore interface {
	Get(ctx context.Context, id uuid.UUID) (contact.Contact, error)
	ListContacts(
		ctx context.Context, query, digits, afterName string, afterID uuid.UUID, limit int,
	) ([]contact.Contact, error)
	ListContactIdentities(ctx context.Context, contactID uuid.UUID) ([]contact.Identity, error)
	ListByIDs(ctx context.Context, ids []uuid.UUID) ([]contact.Contact, error)
}

// TaskStore provides the task reads the graph resolves from.
type TaskStore interface {
	Get(ctx context.Context, id uuid.UUID) (task.Task, error)
	ListForDay(
		ctx context.Context, assigneeID uuid.UUID, dueOn time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListDueBefore(
		ctx context.Context, assigneeID uuid.UUID, dueBefore time.Time, status string, page task.Page,
	) ([]task.Task, error)
	ListForContact(
		ctx context.Context, contactID uuid.UUID, status string, page task.Page,
	) ([]task.Task, error)
}

// Resolver is the root resolver serving the core schema.
type Resolver struct {
	// Version is the reported application version.
	Version string
	// Contacts serves contact reads.
	Contacts ContactStore
	// Tasks serves task reads.
	Tasks TaskStore
}

// Query returns the query resolver set.
func (r *Resolver) Query() graph.QueryResolver {
	return queryResolver{root: r}
}

// Contact returns the contact field resolver set.
func (r *Resolver) Contact() graph.ContactResolver {
	return contactResolver{root: r}
}

// Task returns the task field resolver set.
func (r *Resolver) Task() graph.TaskResolver {
	return taskResolver{root: r}
}

// queryResolver serves the Query root fields.
type queryResolver struct {
	root *Resolver
}

// Version reports the application version.
func (q queryResolver) Version(context.Context) (string, error) {
	return q.root.Version, nil
}
