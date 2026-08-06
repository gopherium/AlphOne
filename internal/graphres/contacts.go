// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/graph/model"
	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/cursor"
)

// errInvalidFirst reports a page size outside the accepted range.
var errInvalidFirst = errors.New("graph: first must be between 1 and 200")

// defaultPageSize and maxPageSize bound a connection page, matching REST.
const (
	defaultPageSize = 50
	maxPageSize     = 200
)

// pageSize resolves the first argument into a page size.
func pageSize(first *int) (int, error) {
	if first == nil {
		return defaultPageSize, nil
	}
	if *first < 1 || *first > maxPageSize {
		return 0, errInvalidFirst
	}
	return *first, nil
}

// stringOf dereferences an optional string argument.
func stringOf(raw *string) string {
	if raw == nil {
		return ""
	}
	return *raw
}

// digitsOf keeps only the decimal digits of a search query.
func digitsOf(query string) string {
	var digits strings.Builder
	for _, r := range query {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	return digits.String()
}

// toContact maps a domain contact to its graph model, id and name only.
func toContact(c contact.Contact) *model.Contact {
	return &model.Contact{ID: c.ID, Name: c.Name}
}

// Contacts pages the contact directory as a connection.
func (q queryResolver) Contacts(
	ctx context.Context, search *string, first *int, after *string,
) (*model.ContactConnection, error) {
	limit, err := pageSize(first)
	if err != nil {
		return nil, err
	}
	afterName, afterID, err := cursor.DecodeContact(stringOf(after))
	if err != nil {
		return nil, err
	}
	term := stringOf(search)
	rows, err := q.root.Contacts.ListContacts(ctx, term, digitsOf(term), afterName, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	return contactConnection(ctx, rows, limit), nil
}

// contactConnection assembles a contact page into a connection, priming the
// request loader with every row.
func contactConnection(ctx context.Context, rows []contact.Contact, limit int) *model.ContactConnection {
	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	edges := make([]*model.ContactEdge, len(rows))
	for i, row := range rows {
		primeContact(ctx, row)
		edges[i] = &model.ContactEdge{Node: toContact(row), Cursor: cursor.EncodeContact(row)}
	}
	pageInfo := &model.PageInfo{HasNextPage: hasNext}
	if len(edges) > 0 {
		pageInfo.StartCursor = &edges[0].Cursor
		pageInfo.EndCursor = &edges[len(edges)-1].Cursor
	}
	return &model.ContactConnection{Edges: edges, PageInfo: pageInfo}
}

// Contact returns one contact by id.
func (q queryResolver) Contact(ctx context.Context, id uuid.UUID) (*model.Contact, error) {
	row, err := q.root.Contacts.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	primeContact(ctx, row)
	return toContact(row), nil
}

// contactResolver serves the Contact field resolvers.
type contactResolver struct {
	root *Resolver
}

// CreatedAt resolves a contact's creation time through the request loader.
func (c contactResolver) CreatedAt(ctx context.Context, obj *model.Contact) (*time.Time, error) {
	row, err := loadContact(ctx, obj.ID)
	if err != nil {
		return nil, err
	}
	createdAt := row.CreatedAt.UTC()
	return &createdAt, nil
}

// Identities resolves a contact's identities.
func (c contactResolver) Identities(ctx context.Context, obj *model.Contact) ([]*model.ContactIdentity, error) {
	rows, err := c.root.Contacts.ListContactIdentities(ctx, obj.ID)
	if err != nil {
		return nil, err
	}
	identities := make([]*model.ContactIdentity, len(rows))
	for i, row := range rows {
		identities[i] = &model.ContactIdentity{
			ID:          row.ID,
			Channel:     string(row.Channel),
			Identifier:  row.Identifier,
			DisplayName: row.DisplayName,
		}
	}
	return identities, nil
}

// Tasks pages the contact's tasks as a connection.
func (c contactResolver) Tasks(
	ctx context.Context, obj *model.Contact, status *string, first *int, after *string,
) (*model.TaskConnection, error) {
	st, page, limit, err := taskPageArgs(status, first, after)
	if err != nil {
		return nil, err
	}
	rows, err := c.root.Tasks.ListForContact(ctx, obj.ID, st, page)
	if err != nil {
		return nil, err
	}
	return taskConnection(rows, limit), nil
}
