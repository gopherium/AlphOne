// SPDX-License-Identifier: Elastic-2.0

package graphroot_test

import (
	"testing"

	"github.com/gopherium/alphone/internal/graphroot"
	"github.com/gopherium/alphone/sdk"
)

// lazyURL never connects, so registration succeeds without a database.
const lazyURL = "postgres://graph:graph@localhost:1/graph"

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

func TestAllReportsTheFirstRegistrationFailure(t *testing.T) {
	t.Parallel()

	plugins, err := graphroot.All(sdk.Deps{DatabaseURL: "://not a database url"})

	if err == nil {
		t.Fatal("All() error = nil, want the registration failure")
	}
	if plugins != nil {
		t.Errorf("All() = %v, want no plugins beside the error", plugins)
	}
}

func TestAllReportsALaterRegistrationFailure(t *testing.T) {
	t.Parallel()

	plugins, err := graphroot.All(sdk.Deps{
		DatabaseURL: lazyURL,
		Getenv: func(name string) string {
			if name == "ALPHONE_WHATSAPP_MEDIA_MAX_BYTES" {
				return "not a byte count"
			}
			return ""
		},
	})

	if err == nil {
		t.Fatal("All() error = nil, want the registration failure")
	}
	if plugins != nil {
		t.Errorf("All() = %v, want no plugins beside the error", plugins)
	}
}
