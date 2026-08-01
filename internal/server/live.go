// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gopherium/gouncer/authkit"

	"github.com/gopherium/alphone/internal/event"
)

// handleEventStream streams published event names as Server-Sent Events.
func (s *server) handleEventStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.live == nil {
			authkit.RespondError(w, http.StatusServiceUnavailable, "live events unavailable")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		controller := http.NewResponseController(w)
		subscription := s.live.Subscribe()
		defer s.live.Unsubscribe(subscription)

		if err := controller.Flush(); err != nil {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		ctx := r.Context()
		for {
			select {
			case <-ctx.Done():
				drainNames(w, controller, subscription)
				return
			case name := <-subscription:
				if err := writeName(w, controller, name); err != nil {
					return
				}
			}
		}
	}
}

// drainNames writes the names still buffered in subscription to the stream.
func drainNames(w http.ResponseWriter, controller *http.ResponseController, subscription chan event.Name) {
	for range len(subscription) {
		if err := writeName(w, controller, <-subscription); err != nil {
			return
		}
	}
}

// writeName writes one event name to the stream as an SSE data frame.
func writeName(w http.ResponseWriter, controller *http.ResponseController, name event.Name) error {
	payload, _ := json.Marshal(struct {
		Event event.Name `json:"event"`
	}{Event: name})
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	_ = controller.Flush()
	return nil
}
