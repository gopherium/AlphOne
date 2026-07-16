// SPDX-License-Identifier: Elastic-2.0

package whatsapp

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// handleStream streams conversation change events to the client as
// Server-Sent Events until the request context ends.
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
		for {
			select {
			case <-ctx.Done():
				drainSubscription(w, controller, subscription)
				return
			case e := <-subscription:
				if err := writeEvent(w, controller, e); err != nil {
					return
				}
			}
		}
	}
}

// drainSubscription writes the events still buffered in subscription to the stream.
func drainSubscription(w http.ResponseWriter, controller *http.ResponseController, subscription chan event) {
	for range len(subscription) {
		if err := writeEvent(w, controller, <-subscription); err != nil {
			return
		}
	}
}

// writeEvent writes one event to the stream as an SSE data frame.
func writeEvent(w http.ResponseWriter, controller *http.ResponseController, e event) error {
	payload, _ := json.Marshal(e)
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	_ = controller.Flush()
	return nil
}
