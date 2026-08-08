// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"context"

	"github.com/gopherium/alphone/internal/event"
)

// Publisher announces domain events.
type Publisher interface {
	Publish(ctx context.Context, frame event.Frame, data map[string]any)
}

// publish announces an event unless the server was built without a
// publisher.
func (s *server) publish(ctx context.Context, frame event.Frame, data map[string]any) {
	if s.events == nil {
		return
	}
	s.events.Publish(ctx, frame, data)
}
