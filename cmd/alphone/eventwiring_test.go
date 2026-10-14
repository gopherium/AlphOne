// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// eventDeclaringPlugin declares the event names it publishes.
type eventDeclaringPlugin struct {
	inertPlugin
	names []string
}

// Events reports the declared event names.
func (p eventDeclaringPlugin) Events() []string { return p.names }

func TestComposeDeclaresThePluginEvents(t *testing.T) {
	t.Parallel()

	deck := eventDeclaringPlugin{inertPlugin{id: "deck"}, []string{"deck.card.drawn"}}

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{deck}, nil
	})

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	if !event.Name("deck.card.drawn").Valid() {
		t.Error("deck.card.drawn reported invalid after compose, want the plugin's event declared")
	}
}

func TestComposeRefusesAPluginEventItCannotClaim(t *testing.T) {
	t.Parallel()

	for claim, tc := range map[string]struct {
		plugins []sdk.Plugin
		named   string
	}{
		"a core event": {
			plugins: []sdk.Plugin{eventDeclaringPlugin{inertPlugin{id: "rogue"}, []string{string(event.TaskCreated)}}},
			named:   "rogue",
		},
		"an event another plugin claimed": {
			plugins: []sdk.Plugin{
				eventDeclaringPlugin{inertPlugin{id: "left"}, []string{"shared.event.fired"}},
				eventDeclaringPlugin{inertPlugin{id: "right"}, []string{"shared.event.fired"}},
			},
			named: "right",
		},
	} {
		t.Run(claim, func(t *testing.T) {
			t.Parallel()

			built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), func(sdk.Deps) ([]sdk.Plugin, error) {
				return tc.plugins, nil
			})
			abandonAtEnd(t, built)

			if !errors.Is(err, event.ErrNameTaken) || !strings.Contains(err.Error(), tc.named) {
				t.Errorf("compose() error = %v, want %s refused naming the plugin %s", err, claim, tc.named)
			}
		})
	}
}

// listenedPublisher returns a plugin publisher over a hub with one default tenant listener, logging into the builder.
func listenedPublisher(logged *strings.Builder) (pluginPublisher, chan event.Name) {
	logger := slog.New(slog.NewTextHandler(logged, nil))
	hub := event.NewHub()
	listener := hub.Subscribe(uuid.Must(uuid.NewV7()), sdk.DefaultTenantID)
	return pluginPublisher{publisher: nudgingPublisher{
		dispatcher: webhook.NewDispatcher(emptyQueue{}, logger),
		worker:     webhook.NewWorker(emptyQueue{}, logger, webhook.AddressGuard{}),
		hub:        hub,
	}, logger: logger}, listener
}

func TestAPluginPublishingANameNoPluginDeclaredIsDropped(t *testing.T) {
	t.Parallel()

	for kind, name := range map[string]string{
		"a name nobody declared": "never.declared.event",
		"a core name":            string(event.TaskCreated),
	} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			var logged strings.Builder
			publisher, listener := listenedPublisher(&logged)

			publisher.Publish(t.Context(), name, map[string]any{"id": "abc"})

			if len(listener) != 0 {
				t.Errorf("the hub carried %d frames for %s, want none", len(listener), kind)
			}
			if !strings.Contains(logged.String(), name) {
				t.Errorf("logged %q, want %s named", logged.String(), name)
			}
		})
	}
}

func TestADeclaredPluginEventReachesTheHub(t *testing.T) {
	t.Parallel()

	if err := event.Declare("relay", "relay.message.passed"); err != nil {
		t.Fatalf("Declare() error = %v, want nil", err)
	}
	var logged strings.Builder
	publisher, listener := listenedPublisher(&logged)

	publisher.Publish(t.Context(), "relay.message.passed", map[string]any{"id": "abc"})

	if len(listener) != 1 {
		t.Errorf("the hub carried %d frames for a declared plugin event, want one", len(listener))
	}
}
