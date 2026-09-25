// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/hashicorp/golang-lru/v2/simplelru"

	"github.com/gopherium/alphone/sdk"
)

// contactEntity names the GraphQL type every defined field hangs on.
const contactEntity = "Contact"

// loader reads the calling tenant's live catalogue.
type loader interface {
	liveDefinitions(ctx context.Context) ([]Definition, error)
}

// view is one tenant's immutable reading of its live definitions.
type view struct {
	stamp  uint64
	read   time.Time
	fields []sdk.GraphField
	kinds  map[string]kind
}

// flight is one read of a tenant's catalogue, shared by every caller missing that tenant meanwhile.
type flight struct {
	done      chan struct{}
	stop      context.CancelFunc
	waiting   int
	overtaken bool
	view      *view
	err       error
}

// catalog holds the views of the tenants served most recently.
type catalog struct {
	loader  loader
	refresh time.Duration
	now     func() time.Time
	mu      sync.Mutex
	views   *simplelru.LRU[uuid.UUID, *view]
	flights map[uuid.UUID]*flight
	stamped uint64
}

// newCatalog returns a catalogue reading through the given loader, holding up to held tenants for refresh each.
func newCatalog(source loader, held int, refresh time.Duration) *catalog {
	views, _ := simplelru.NewLRU[uuid.UUID, *view](cmp.Or(max(held, 0), sdk.DefaultTenantsHeld), nil)
	return &catalog{
		loader:  source,
		refresh: cmp.Or(max(refresh, 0), sdk.DefaultTenantsRefresh),
		now:     time.Now,
		views:   views,
		flights: map[uuid.UUID]*flight{},
	}
}

// viewFor returns the calling tenant's view, reading it when none is held, it is too old or a forget overtook the read.
func (c *catalog) viewFor(ctx context.Context) (*view, error) {
	tenant := sdk.TenantOrDefault(ctx)
	for {
		held, shared := c.join(ctx, tenant)
		if held != nil {
			return held, nil
		}
		select {
		case <-shared.done:
			if !shared.overtaken {
				return shared.view, shared.err
			}
		case <-ctx.Done():
			c.leave(tenant, shared)
			return nil, ctx.Err()
		}
	}
}

// join returns the tenant's fresh view, or else the read of it under way, starting one when none is.
func (c *catalog) join(ctx context.Context, tenant uuid.UUID) (*view, *flight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if held, ok := c.views.Get(tenant); ok && c.now().Sub(held.read) < c.refresh {
		return held, nil
	}
	shared, ok := c.flights[tenant]
	if !ok {
		reading, stop := context.WithCancel(context.WithoutCancel(ctx))
		shared = &flight{done: make(chan struct{}), stop: stop}
		c.flights[tenant] = shared
		go c.fly(reading, tenant, shared)
	}
	shared.waiting++
	return nil, shared
}

// leave drops one caller from a shared read, stopping the read once no caller waits on it.
func (c *catalog) leave(tenant uuid.UUID, shared *flight) {
	c.mu.Lock()
	defer c.mu.Unlock()
	shared.waiting--
	if shared.waiting == 0 {
		shared.stop()
		c.detach(tenant, shared)
	}
}

// fly reads the tenant's catalogue and hands the result to every caller sharing the flight.
func (c *catalog) fly(ctx context.Context, tenant uuid.UUID, shared *flight) {
	read, err := c.read(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	shared.stop()
	current := c.detach(tenant, shared)
	if err == nil {
		read.stamp = c.stampFor(tenant, read.fields)
		if current {
			c.views.Add(tenant, read)
		}
	}
	shared.view, shared.err = read, err
	close(shared.done)
}

// detach keeps later callers from joining the given read, reporting whether it was the tenant's current one.
func (c *catalog) detach(tenant uuid.UUID, shared *flight) bool {
	if c.flights[tenant] != shared {
		return false
	}
	delete(c.flights, tenant)
	return true
}

// stampFor returns the stamp of the tenant's held view when that view holds the same fields, a new stamp otherwise.
func (c *catalog) stampFor(tenant uuid.UUID, fields []sdk.GraphField) uint64 {
	if held, ok := c.views.Peek(tenant); ok && slices.Equal(held.fields, fields) {
		return held.stamp
	}
	c.stamped++
	return c.stamped
}

// read builds an unstamped view of the calling tenant's live definitions.
func (c *catalog) read(ctx context.Context) (*view, error) {
	definitions, err := c.loader.liveDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	next := &view{
		read:   c.now(),
		fields: make([]sdk.GraphField, 0, len(definitions)),
		kinds:  make(map[string]kind, len(definitions)),
	}
	for _, definition := range definitions {
		next.fields = append(next.fields, sdk.GraphField{
			Entity: contactEntity,
			Name:   definition.Name,
			Type:   definition.Kind.scalar(),
		})
		next.kinds[definition.Name] = definition.Kind
	}
	return next, nil
}

// forget drops the calling tenant's view and overtakes any read of it under way.
func (c *catalog) forget(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.drop(sdk.TenantOrDefault(ctx))
}

// drop removes the tenant's view and stops any read of it under way, sending that read's callers to read again.
func (c *catalog) drop(tenant uuid.UUID) {
	if shared, ok := c.flights[tenant]; ok {
		shared.overtaken = true
		shared.stop()
		delete(c.flights, tenant)
	}
	c.views.Remove(tenant)
}
