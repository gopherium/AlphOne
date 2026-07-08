// SPDX-License-Identifier: AGPL-3.0-or-later

package whatsapp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleStream streams conversation change events to the client as
// Server-Sent Events until the client disconnects.
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

		for {
			select {
			case <-r.Context().Done():
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
