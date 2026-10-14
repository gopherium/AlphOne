// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/event"
)

func TestWebhookEventsListsTheCoreEventsThenEveryDeclaredOne(t *testing.T) {
	t.Parallel()

	if err := event.Declare("graphprobe", "graphprobe.thing.happened"); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}
	h := newMutationHarness(t)
	var listed struct {
		WebhookEvents []string `json:"webhookEvents"`
	}

	h.client.MustPost(`{ webhookEvents }`, &listed)

	core := make([]string, 0, len(event.CoreNames()))
	for _, name := range event.CoreNames() {
		core = append(core, string(name))
	}
	if len(listed.WebhookEvents) < len(core) || !slices.Equal(listed.WebhookEvents[:len(core)], core) {
		t.Errorf("webhookEvents = %v, want the core events %v first", listed.WebhookEvents, core)
	}
	if !slices.Contains(listed.WebhookEvents, "graphprobe.thing.happened") {
		t.Errorf("webhookEvents = %v, want the declared plugin event listed", listed.WebhookEvents)
	}
}
