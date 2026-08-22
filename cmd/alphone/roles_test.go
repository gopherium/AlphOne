// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync/atomic"
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

// stoppingPlugin records whether the host stopped it.
type stoppingPlugin struct {
	silentPlugin
	stopped *atomic.Bool
}

// Stop records that the host stopped the plugin.
func (p stoppingPlugin) Stop(context.Context) error {
	p.stopped.Store(true)
	return nil
}

func TestRunStopsThePluginHostWhenTheGraphCannotCompose(t *testing.T) {
	t.Parallel()

	var stopped atomic.Bool
	standing := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{stoppingPlugin{stopped: &stopped}}, nil
	}

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": testDatabaseURL(t),
	}), io.Discard, standing)

	if err == nil || !strings.Contains(err.Error(), "compose graph root") {
		t.Fatalf("run() error = %v, want the compose graph root failure", err)
	}
	if !stopped.Load() {
		t.Error("the plugin host was left running, want it stopped before the failure returns")
	}
}

func TestDeclaringPluginRolesTeachesTheRegistryBeforeACommandParsesOne(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	declaring := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{
			rolePlugin{declared: []sdk.RoleDeclaration{
				{Name: "steward", Capabilities: []string{"manage_users"}},
			}},
		}, nil
	}

	if err := declarePluginRoles(registry, testGetenv(nil), declaring); err != nil {
		t.Fatalf("declarePluginRoles() error = %v, want nil", err)
	}

	if _, err := registry.Parse("steward"); err != nil {
		t.Errorf("Parse(steward) error = %v, want a command able to name a declared role", err)
	}
}

func TestEveryRoleWritingSubcommandRefusesAPluginItCannotRegister(t *testing.T) {
	t.Parallel()

	failing := func(sdk.Deps) ([]sdk.Plugin, error) { return nil, errPluginMigrate }

	for _, name := range []string{"createadmin", "grantrole"} {
		err := dispatch(t.Context(), []string{name, "-role", "admin"}, failing)

		if !errors.Is(err, errPluginMigrate) {
			t.Errorf("dispatch(%q) error = %v, want the registrar failure refused before any role is parsed",
				name, err)
		}
	}
}

func TestDeclaringPluginRolesReportsARegistrarThatFails(t *testing.T) {
	t.Parallel()

	failing := func(sdk.Deps) ([]sdk.Plugin, error) { return nil, errPluginMigrate }

	err := declarePluginRoles(role.NewRegistry(), testGetenv(nil), failing)

	if !errors.Is(err, errPluginMigrate) {
		t.Errorf("declarePluginRoles() error = %v, want the registrar failure reported", err)
	}
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
