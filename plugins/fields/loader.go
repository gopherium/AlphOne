// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"cmp"
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/vikstrous/dataloadgen"

	"github.com/gopherium/alphone/sdk"
)

// valueLoaderKey keys the contact value loader in the request scope.
type valueLoaderKey struct{}

// valueLoader returns the request's value loader, building it once.
func (p *Plugin) valueLoader(ctx context.Context) (*dataloadgen.Loader[uuid.UUID, map[string]any], error) {
	return sdk.ScopedValue(ctx, valueLoaderKey{}, func() *dataloadgen.Loader[uuid.UUID, map[string]any] {
		return dataloadgen.NewLoader(p.fetchValues, dataloadgen.WithWait(cmp.Or(p.batchWait, time.Millisecond)))
	})
}

// fetchValues loads the value bags of a batch of contacts.
func (p *Plugin) fetchValues(ctx context.Context, ids []uuid.UUID) ([]map[string]any, []error) {
	results := make([]map[string]any, len(ids))
	errs := make([]error, len(ids))
	held, err := p.store.valuesFor(ctx, ids)
	if err != nil {
		for i := range errs {
			errs[i] = err
		}
		return results, errs
	}
	for i, id := range ids {
		results[i] = held[id]
	}
	return results, errs
}

// loadValues returns one contact's value bag through the request loader.
func (p *Plugin) loadValues(ctx context.Context, id uuid.UUID) (map[string]any, error) {
	loader, err := p.valueLoader(ctx)
	if err != nil {
		return nil, err
	}
	return loader.Load(ctx, id)
}
