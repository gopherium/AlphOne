// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// defaultStreamLifetime is how long an SSE connection stays open.
const defaultStreamLifetime = 5 * time.Minute

// handleStream streams conversation change events to the client as
// Server-Sent Events until the client disconnects or the connection
// reaches its lifetime.
func (p *Plugin) handleStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		controller := http.NewResponseController(w)
		subscription := p.events.subscribe()
		defer p.events.unsubscribe(subscription)

		if err := controller.Flush(); err != nil {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		if p.streamLifetime > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, p.streamLifetime)
			defer cancel()
		}

		for {
			select {
			case <-ctx.Done():
				return
			case e := <-subscription:
				payload, _ := json.Marshal(e)
				if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
					return
				}
				_ = controller.Flush()
			}
		}
	}
}
