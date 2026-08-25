// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/sdk"
)

// subscriptionBuffer is how many names a subscription can lag behind.
const subscriptionBuffer = 8

// errLiveUnavailable reports a graph built without a broadcaster.
var errLiveUnavailable = errors.New("graphres: live events unavailable")

// SubscriptionResolvers serves the Subscription root fields.
type SubscriptionResolvers struct {
	root *Resolver
}

// SubscriptionResolvers returns the core Subscription resolver set.
func (r *Resolver) SubscriptionResolvers() SubscriptionResolvers {
	return SubscriptionResolvers{root: r}
}

// CoreEvent streams the names of the events the subscriber may see.
func (s SubscriptionResolvers) CoreEvent(ctx context.Context) (<-chan string, error) {
	if s.root.Live == nil {
		return nil, sdk.GraphError{Code: "UNAVAILABLE", Err: errLiveUnavailable}
	}
	names := make(chan string, subscriptionBuffer)
	go s.pumpNames(ctx, authkit.IdentityFromContext(ctx).ID, names)
	return names, nil
}

// pumpNames forwards the user's frames onto names until the context ends.
func (s SubscriptionResolvers) pumpNames(ctx context.Context, user uuid.UUID, names chan<- string) {
	frames := s.root.Live.Subscribe(user, sdk.TenantOrDefault(ctx))
	go func() {
		<-ctx.Done()
		s.root.Live.Unsubscribe(frames)
	}()
	defer close(names)
	for name := range frames {
		if !forwardName(ctx, names, name) {
			return
		}
	}
}

// forwardName sends name unless the context ended first.
func forwardName(ctx context.Context, names chan<- string, name event.Name) bool {
	select {
	case names <- string(name):
		return true
	case <-ctx.Done():
		return false
	}
}
