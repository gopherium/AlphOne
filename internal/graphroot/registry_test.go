// SPDX-License-Identifier: Elastic-2.0

package graphroot_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/sdk"
)

// lazyURL never connects, so registration succeeds without a database.
const lazyURL = "postgres://graph:graph@localhost:1/graph"

// stopGrace bounds the stop of every plugin a test registered.
const stopGrace = 10 * time.Second

// stopRegistered stops every plugin a test registered once the test ends.
func stopRegistered(t *testing.T, registered []sdk.Plugin) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), stopGrace)
		defer cancel()
		for _, plugin := range registered {
			if err := plugin.Stop(ctx); err != nil {
				t.Errorf("stopping plugin %s: %v", plugin.ID(), err)
			}
		}
	})
}

// idsOf returns the id of every plugin in order.
func idsOf(plugins []sdk.Plugin) []string {
	ids := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		ids = append(ids, plugin.ID())
	}
	return ids
}

func TestAllRegistersEveryPlugin(t *testing.T) {
	t.Parallel()

	plugins, err := graphroot.All(sdk.Deps{DatabaseURL: lazyURL})

	if err != nil {
		t.Fatalf("All() error = %v, want nil", err)
	}
	if len(plugins) == 0 {
		t.Fatal("All() returned no plugins, want every registered plugin")
	}
	for _, plugin := range plugins {
		t.Cleanup(func() { _ = plugin.Stop(t.Context()) })
		if plugin.ID() == "" {
			t.Error("a registered plugin reports no id")
		}
	}
}

func TestAllNamesEachPluginThatCannotReadTheDatabaseURL(t *testing.T) {
	t.Parallel()

	plugins, err := graphroot.All(sdk.Deps{DatabaseURL: "://not a database url"})
	stopRegistered(t, plugins)

	ids := idsOf(plugins)
	for _, id := range []string{"fields", "importer", "whatsapp"} {
		if err == nil || !strings.Contains(err.Error(), "plugin "+id+": ") {
			t.Errorf("All() error = %v, want the %s plugin named", err, id)
		}
		if slices.Contains(ids, id) {
			t.Errorf("All() = %v, want the %s plugin absent beside its failure", ids, id)
		}
	}
}

func TestAllReturnsThePluginsThatRegisteredBesideALaterFailure(t *testing.T) {
	t.Parallel()

	plugins, err := graphroot.All(sdk.Deps{
		DatabaseURL: lazyURL,
		Env: sdk.Env{Prefix: "ALPHONE_", Getenv: func(name string) string {
			if name == "ALPHONE_WHATSAPP_MEDIA_MAX_BYTES" {
				return "not a byte count"
			}
			return ""
		}},
	})
	stopRegistered(t, plugins)

	if err == nil || !strings.Contains(err.Error(), "plugin whatsapp: ") {
		t.Fatalf("All() error = %v, want the whatsapp plugin named", err)
	}
	if ids := idsOf(plugins); len(ids) == 0 || slices.Contains(ids, "whatsapp") {
		t.Errorf("All() = %v, want the plugins that registered and the whatsapp plugin absent", ids)
	}
}
