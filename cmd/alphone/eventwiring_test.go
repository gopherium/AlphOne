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

func TestComposeRefusesAPluginEventCoreOwns(t *testing.T) {
	t.Parallel()

	rogue := eventDeclaringPlugin{inertPlugin{id: "rogue"}, []string{string(event.TaskCreated)}}

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{rogue}, nil
	})
	abandonAtEnd(t, built)

	if !errors.Is(err, event.ErrNameTaken) || !strings.Contains(err.Error(), "rogue") {
		t.Errorf("compose() error = %v, want the core event refused naming the plugin", err)
	}
}

func TestComposeRefusesTwoPluginsClaimingOneEvent(t *testing.T) {
	t.Parallel()

	left := eventDeclaringPlugin{inertPlugin{id: "left"}, []string{"shared.event.fired"}}
	right := eventDeclaringPlugin{inertPlugin{id: "right"}, []string{"shared.event.fired"}}

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{left, right}, nil
	})
	abandonAtEnd(t, built)

	if !errors.Is(err, event.ErrNameTaken) || !strings.Contains(err.Error(), "right") {
		t.Errorf("compose() error = %v, want the second claim refused naming its plugin", err)
	}
}

func TestAPluginEventNobodyDeclaredIsDropped(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	logger := slog.New(slog.NewTextHandler(&logged, nil))
	hub := event.NewHub()
	listener := hub.Subscribe(uuid.Must(uuid.NewV7()), sdk.DefaultTenantID)
	publisher := pluginPublisher{publisher: nudgingPublisher{
		dispatcher: webhook.NewDispatcher(emptyQueue{}, logger),
		worker:     webhook.NewWorker(emptyQueue{}, logger, webhook.AddressGuard{}),
		hub:        hub,
	}, logger: logger}

	publisher.Publish(t.Context(), "never.declared.event", map[string]any{"id": "abc"})

	if len(listener) != 0 {
		t.Errorf("the hub carried %d frames for an undeclared event, want none", len(listener))
	}
	if !strings.Contains(logged.String(), "never.declared.event") {
		t.Errorf("logged %q, want the undeclared event named", logged.String())
	}
}
