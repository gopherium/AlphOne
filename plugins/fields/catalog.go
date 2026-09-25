// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"cmp"
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	lru "github.com/hashicorp/golang-lru/v2"

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

// catalog holds the views of the tenants served most recently.
type catalog struct {
	loader  loader
	views   *lru.Cache[uuid.UUID, *view]
	refresh time.Duration
	now     func() time.Time
	stamps  atomic.Uint64
	forgets atomic.Uint64
	storing sync.Mutex
}

// newCatalog returns a catalogue reading through the given loader, holding up to held tenants for refresh each.
func newCatalog(source loader, held int, refresh time.Duration) *catalog {
	views, _ := lru.New[uuid.UUID, *view](cmp.Or(max(held, 0), sdk.DefaultTenantsHeld))
	return &catalog{
		loader:  source,
		views:   views,
		refresh: cmp.Or(max(refresh, 0), sdk.DefaultTenantsRefresh),
		now:     time.Now,
	}
}

// viewFor returns the calling tenant's view, reading it when none is held or the held one is too old.
func (c *catalog) viewFor(ctx context.Context) (*view, error) {
	tenant := sdk.TenantOrDefault(ctx)
	if held, ok := c.views.Get(tenant); ok && c.now().Sub(held.read) < c.refresh {
		return held, nil
	}
	generation := c.forgets.Load()
	read, err := c.read(ctx)
	if err != nil {
		return nil, err
	}
	c.storing.Lock()
	defer c.storing.Unlock()
	if c.forgets.Load() == generation {
		c.views.Add(tenant, read)
	}
	return read, nil
}

// read builds a freshly stamped view of the calling tenant's live definitions.
func (c *catalog) read(ctx context.Context) (*view, error) {
	definitions, err := c.loader.liveDefinitions(ctx)
	if err != nil {
		return nil, err
	}
	next := &view{
		stamp:  c.stamps.Add(1),
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

// forget drops the calling tenant's view, keeping any read already under way out of the cache.
func (c *catalog) forget(ctx context.Context) {
	c.storing.Lock()
	defer c.storing.Unlock()
	c.forgets.Add(1)
	c.views.Remove(sdk.TenantOrDefault(ctx))
}
