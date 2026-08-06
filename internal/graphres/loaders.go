// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"github.com/gopherium/alphone/internal/contact"
)

// errNoLoaders reports a resolver running outside a loader carrying request.
var errNoLoaders = errors.New("graphres: no loaders on context")

// loadersKey keys the request loaders on the context.
type loadersKey struct{}

// loaders carries the per request batch loaders of the graph.
type loaders struct {
	contacts *dataloadgen.Loader[uuid.UUID, contact.Contact]
}

// WithLoaders installs fresh per request loaders on ctx.
func (r *Resolver) WithLoaders(ctx context.Context) context.Context {
	bundle := &loaders{
		contacts: dataloadgen.NewLoader(r.fetchContacts, dataloadgen.WithWait(time.Millisecond)),
	}
	return context.WithValue(ctx, loadersKey{}, bundle)
}

// fetchContacts loads a batch of contacts by id, one error per missing id.
func (r *Resolver) fetchContacts(ctx context.Context, ids []uuid.UUID) ([]contact.Contact, []error) {
	rows, err := r.Contacts.ListByIDs(ctx, ids)
	if err != nil {
		results := make([]contact.Contact, len(ids))
		errs := make([]error, len(ids))
		for i := range errs {
			errs[i] = err
		}
		return results, errs
	}
	return matchByID(ids, rows)
}

// matchByID pairs each requested id with its row, ErrNotFound for absent ones.
func matchByID(ids []uuid.UUID, rows []contact.Contact) ([]contact.Contact, []error) {
	byID := make(map[uuid.UUID]contact.Contact, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	results := make([]contact.Contact, len(ids))
	errs := make([]error, len(ids))
	for i, id := range ids {
		row, ok := byID[id]
		if !ok {
			errs[i] = contact.ErrNotFound
			continue
		}
		results[i] = row
	}
	return results, errs
}

// requestLoaders returns the loaders installed on ctx.
func requestLoaders(ctx context.Context) (*loaders, error) {
	bundle, ok := ctx.Value(loadersKey{}).(*loaders)
	if !ok {
		return nil, errNoLoaders
	}
	return bundle, nil
}

// loadContact returns one contact through the request loader.
func loadContact(ctx context.Context, id uuid.UUID) (contact.Contact, error) {
	bundle, err := requestLoaders(ctx)
	if err != nil {
		return contact.Contact{}, err
	}
	return bundle.contacts.Load(ctx, id)
}

// primeContact seeds the request loader with an already fetched row.
func primeContact(ctx context.Context, c contact.Contact) {
	bundle, err := requestLoaders(ctx)
	if err != nil {
		return
	}
	bundle.contacts.Prime(c.ID, c)
}
