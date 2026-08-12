// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/gopherium/alphone/sdk"
)

// contactEntity names the GraphQL type every defined field hangs on.
const contactEntity = "Contact"

// loader reads the live catalogue the graph serves.
type loader interface {
	liveDefinitions(ctx context.Context) ([]Definition, error)
}

// view is one immutable reading of the catalogue.
type view struct {
	version uint64
	fields  []sdk.GraphField
	names   map[string]bool
}

// catalog holds the versioned view of the live definitions.
type catalog struct {
	loader  loader
	live    atomic.Pointer[view]
	reading sync.Mutex
}

// newCatalog returns an empty catalogue reading through the given loader.
func newCatalog(source loader) *catalog {
	held := &catalog{loader: source}
	held.live.Store(&view{names: map[string]bool{}})
	return held
}

// reload replaces the held view with a fresh read.
func (c *catalog) reload(ctx context.Context) error {
	c.reading.Lock()
	defer c.reading.Unlock()
	definitions, err := c.loader.liveDefinitions(ctx)
	if err != nil {
		return err
	}
	next := &view{
		version: c.live.Load().version + 1,
		fields:  make([]sdk.GraphField, 0, len(definitions)),
		names:   make(map[string]bool, len(definitions)),
	}
	for _, definition := range definitions {
		next.fields = append(next.fields, sdk.GraphField{
			Entity: contactEntity,
			Name:   definition.Name,
			Type:   definition.Kind.scalar(),
		})
		next.names[definition.Name] = true
	}
	c.live.Store(next)
	return nil
}

// snapshot reports the catalogue version beside the fields the graph serves.
func (c *catalog) snapshot() (uint64, []sdk.GraphField) {
	held := c.live.Load()
	return held.version, held.fields
}

// holds reports whether a live definition carries the given name.
func (c *catalog) holds(name string) bool {
	return c.live.Load().names[name]
}
