// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// silentPlugin is a plugin declaring no roles.
type silentPlugin struct{}

// ID names the plugin.
func (silentPlugin) ID() string { return "silent" }

// Start does nothing.
func (silentPlugin) Start(context.Context) error { return nil }

// Stop does nothing.
func (silentPlugin) Stop(context.Context) error { return nil }

// rolePlugin is a plugin declaring roles, standing in for one the host wires.
type rolePlugin struct {
	silentPlugin
	declared []sdk.RoleDeclaration
}

// ID names the plugin.
func (rolePlugin) ID() string { return "steward" }

// Roles returns the roles the plugin declares.
func (p rolePlugin) Roles() []sdk.RoleDeclaration {
	return p.declared
}

func TestDeclareRolesGrantsEveryDeclarationAPluginMakes(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registered := []sdk.Plugin{
		silentPlugin{},
		rolePlugin{declared: []sdk.RoleDeclaration{
			{Name: "steward", Capabilities: []string{"manage_users", "manage_reports"}},
			{Name: "admin", Capabilities: []string{"manage_reports"}},
		}},
	}

	if err := declareRoles(registry, registered); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil", err)
	}

	if !registry.Can("steward", "manage_reports") {
		t.Error("Can(steward, manage_reports) = false, want the declared role held")
	}
	if !registry.Can(role.Admin, "manage_reports") {
		t.Error("Can(admin, manage_reports) = false, want the core role widened")
	}
	if got := registry.Privileged(); !slices.Equal(got, []string{"admin", "steward"}) {
		t.Errorf("Privileged() = %v, want the declared role counted as cover", got)
	}
}

func TestRunRefusesAPluginDeclaringANamelessRole(t *testing.T) {
	t.Parallel()

	nameless := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{
			rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"manage_reports"}}}},
		}, nil
	}

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
	}), io.Discard, nameless)

	if !errors.Is(err, role.ErrEmptyRole) {
		t.Fatalf("run() error = %v, want ErrEmptyRole refusing the wiring", err)
	}
}

func TestDeclareRolesRefusesADeclarationWithNoName(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registered := []sdk.Plugin{
		rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"manage_reports"}}}},
	}

	err := declareRoles(registry, registered)

	if !errors.Is(err, role.ErrEmptyRole) {
		t.Errorf("declareRoles() error = %v, want ErrEmptyRole surfaced at wiring", err)
	}
}
