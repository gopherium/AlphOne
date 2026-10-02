// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"cmp"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/sdk"
)

// contactLoaderKey keys the core contact loader in the request scope.
type contactLoaderKey struct{}

// contactLoader returns the request's contact loader, building it once.
func (r *Resolver) contactLoader(ctx context.Context) (*dataloadgen.Loader[uuid.UUID, contact.Contact], error) {
	return sdk.ScopedValue(ctx, contactLoaderKey{}, func() *dataloadgen.Loader[uuid.UUID, contact.Contact] {
		return dataloadgen.NewLoader(r.fetchContacts, dataloadgen.WithWait(cmp.Or(r.BatchWait, time.Millisecond)))
	})
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

// loadContact returns one contact through the request loader.
func (r *Resolver) loadContact(ctx context.Context, id uuid.UUID) (contact.Contact, error) {
	loader, err := r.contactLoader(ctx)
	if err != nil {
		return contact.Contact{}, err
	}
	return loader.Load(ctx, id)
}

// identityLoaderKey keys the core identity loader in the request scope.
type identityLoaderKey struct{}

// identityLoader returns the request's uncached identity loader, building it once.
func (r *Resolver) identityLoader(ctx context.Context) (*dataloadgen.Loader[uuid.UUID, []contact.Identity], error) {
	return sdk.ScopedValue(ctx, identityLoaderKey{}, func() *dataloadgen.Loader[uuid.UUID, []contact.Identity] {
		return dataloadgen.NewLoader(r.fetchIdentities,
			dataloadgen.WithWait(cmp.Or(r.BatchWait, time.Millisecond)), dataloadgen.WithoutCache())
	})
}

// fetchIdentities loads the identities of a batch of contacts, one list per id.
func (r *Resolver) fetchIdentities(ctx context.Context, ids []uuid.UUID) ([][]contact.Identity, []error) {
	results := make([][]contact.Identity, len(ids))
	errs := make([]error, len(ids))
	rows, err := r.Contacts.ListContactIdentities(ctx, ids)
	if err != nil {
		for i := range errs {
			errs[i] = err
		}
		return results, errs
	}
	held := make(map[uuid.UUID][]contact.Identity, len(ids))
	for _, row := range rows {
		held[row.ContactID] = append(held[row.ContactID], row)
	}
	for i, id := range ids {
		results[i] = held[id]
	}
	return results, errs
}

// loadIdentities returns one contact's identities through the request loader.
func (r *Resolver) loadIdentities(ctx context.Context, id uuid.UUID) ([]contact.Identity, error) {
	loader, err := r.identityLoader(ctx)
	if err != nil {
		return nil, err
	}
	return loader.Load(ctx, id)
}

// primeContact seeds the request loader with an already fetched row.
func (r *Resolver) primeContact(ctx context.Context, c contact.Contact) {
	loader, err := r.contactLoader(ctx)
	if err != nil {
		return
	}
	loader.Prime(c.ID, c)
}
