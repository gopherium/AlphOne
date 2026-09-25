// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/sdk"
)

func TestTheTenantBoundsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.tenants.held != sdk.DefaultTenantsHeld || held.tenants.refresh != sdk.DefaultTenantsRefresh {
		t.Errorf("tenants = %+v, want the SDK defaults", held.tenants)
	}
}

func TestTheTenantBoundsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
		"ALPHONE_TENANTS_HELD":    "8",
		"ALPHONE_TENANTS_REFRESH": "30s",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.tenants.held != 8 || held.tenants.refresh != 30*time.Second {
		t.Errorf("tenants = %+v, want 8 tenants refreshed every 30s", held.tenants)
	}
}

func TestTheTenantBoundsRefuseAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name string
		raw  string
	}{
		"no tenants held":       {"ALPHONE_TENANTS_HELD", "0"},
		"fewer than none held":  {"ALPHONE_TENANTS_HELD", "-1"},
		"a held count in words": {"ALPHONE_TENANTS_HELD", "many"},
		"no refresh at all":     {"ALPHONE_TENANTS_REFRESH", "0s"},
		"a negative refresh":    {"ALPHONE_TENANTS_REFRESH", "-1m"},
		"a refresh in words":    {"ALPHONE_TENANTS_REFRESH", "a minute"},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				tt.name:                tt.raw,
			}))

			if err == nil || !strings.Contains(err.Error(), tt.name) {
				t.Errorf("loadRunConfig() with %s=%q error = %v, want the value refused by name", tt.name, tt.raw, err)
			}
		})
	}
}

func TestTheServerConfigCarriesTheTenantBound(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    "postgres://localhost/x",
		"ALPHONE_TENANTS_HELD":    "8",
		"ALPHONE_TRUSTED_PROXIES": "10.0.0.0/8",
		"ALPHONE_DEV_GRAPHIQL":    "1",
	}))
	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}

	cfg := held.serverConfig()

	if cfg.TenantsHeld != 8 {
		t.Errorf("TenantsHeld = %d, want 8", cfg.TenantsHeld)
	}
	if len(cfg.TrustedProxies) != 1 || !cfg.GraphiQL {
		t.Errorf("server config = %+v, want the trusted proxies and GraphiQL carried too", cfg)
	}
}

func TestRunHandsPluginsTheTenantBounds(t *testing.T) {
	t.Parallel()

	var handed sdk.Deps
	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":    testDatabaseURL(t),
		"ALPHONE_TENANTS_HELD":    "8",
		"ALPHONE_TENANTS_REFRESH": "30s",
	}), io.Discard, capturingPlugins(&handed))

	if !errors.Is(err, errCaptured) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errCaptured)
	}
	if handed.TenantsHeld != 8 || handed.TenantsRefresh != 30*time.Second {
		t.Errorf("Deps = %d tenants every %v, want 8 every 30s", handed.TenantsHeld, handed.TenantsRefresh)
	}
}
