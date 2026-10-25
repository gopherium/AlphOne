// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/framework/gonsole/testkit"

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

// corePlugin is a role plugin under the id the core grants its roles under.
type corePlugin struct{ rolePlugin }

// ID names the plugin after the core source.
func (corePlugin) ID() string { return role.Source }

// silentCorePlugin is a plugin declaring no roles under the id the core grants its roles under.
type silentCorePlugin struct{ silentPlugin }

// ID names the plugin after the core source.
func (silentCorePlugin) ID() string { return role.Source }

// idRolePlugin is a role plugin under the id it carries.
type idRolePlugin struct {
	rolePlugin
	id string
}

// ID names the plugin after the id it carries.
func (p idRolePlugin) ID() string { return p.id }

// topRolePlugin is a role plugin naming the deployment's top role.
type topRolePlugin struct {
	idRolePlugin
	top string
}

// TopRole names the role the plugin answers as the top role.
func (p topRolePlugin) TopRole() string { return p.top }

// silentTopRolePlugin is a plugin naming a top role while declaring no roles.
type silentTopRolePlugin struct{ silentPlugin }

// TopRole names a role the plugin never declares.
func (silentTopRolePlugin) TopRole() string { return "owner" }

// ownerRoles is the owner role a plugin declares with the core capabilities.
var ownerRoles = []sdk.RoleDeclaration{{Name: "owner", Capabilities: []string{role.ManageUsers, role.ManageWebhooks}}}

// declaring returns a plugin under the id declaring the roles.
func declaring(id string, declared []sdk.RoleDeclaration) idRolePlugin {
	return idRolePlugin{rolePlugin: rolePlugin{declared: declared}, id: id}
}

// naming returns a plugin under the id declaring the roles and naming the top role.
func naming(id, top string, declared []sdk.RoleDeclaration) topRolePlugin {
	return topRolePlugin{idRolePlugin: declaring(id, declared), top: top}
}

// reportsPlugin returns a plugin under the reports id giving admin a capability of its own.
func reportsPlugin() idRolePlugin {
	return declaring("reports", []sdk.RoleDeclaration{
		{Name: role.Admin.String(), Capabilities: []string{"reports.export_reports"}},
	})
}

// flippingTopRolePlugin is a role plugin naming one top role on its first call and another after it.
type flippingTopRolePlugin struct {
	idRolePlugin
	first, later string
	calls        *atomic.Int32
}

// TopRole answers the first role on the first call and the later role on every call after it.
func (p flippingTopRolePlugin) TopRole() string {
	if p.calls.Add(1) == 1 {
		return p.first
	}
	return p.later
}

// keepsOnlyTheCoreRoles fails t unless the registry holds the core roles with their capabilities and nothing else.
func keepsOnlyTheCoreRoles(t *testing.T, registry *goncierge.Registry) {
	t.Helper()
	core := role.NewRegistry()
	if got, want := registry.Roles(), core.Roles(); !slices.Equal(got, want) {
		t.Errorf("Roles() = %v, want the core roles %v alone, the start refused before any role is declared", got, want)
	}
	for _, name := range core.Roles() {
		if got, want := registry.CapabilitiesOf(name), core.CapabilitiesOf(name); !slices.Equal(got, want) {
			t.Errorf("CapabilitiesOf(%s) = %v, want the core capabilities %v alone", name, got, want)
		}
	}
}

// errStopFailed is the failure the stopping plugin's stop answers.
var errStopFailed = errors.New("stop failed")

// stoppingPlugin records what its stop saw and answers a failed stop.
type stoppingPlugin struct {
	silentPlugin
	stopped *atomic.Pointer[stopSeen]
}

// Stop records what the host's stop context held and fails.
func (p stoppingPlugin) Stop(ctx context.Context) error {
	seen := seenBy(ctx)
	p.stopped.Store(&seen)
	return errStopFailed
}

// stoppedUnderTheStopGrace checks that run stopped the plugin under the stop grace and joined the failed stop.
func stoppedUnderTheStopGrace(t *testing.T, err error, stopped *atomic.Pointer[stopSeen]) {
	t.Helper()
	if !errors.Is(err, errStopFailed) {
		t.Errorf("run() error = %v, want the stop failure %q joined", err, errStopFailed)
	}
	seen := stopped.Load()
	if seen == nil {
		t.Fatal("the plugin was left running, want it stopped before the failure returns")
	}
	if !underTheStopGrace(*seen) {
		t.Errorf("the plugin stopped with live = %v and %v left, want a live context holding most of the %v stop grace",
			seen.live, seen.left, shutdownStopGrace)
	}
}

func TestRunStopsThePluginsThatRegisteredWhenAnotherFails(t *testing.T) {
	t.Parallel()

	var stopped atomic.Pointer[stopSeen]
	partly := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{stoppingPlugin{stopped: &stopped}}, errRegistration
	}

	err := run(t.Context(), stopGraceEnv(t), io.Discard, partly)

	if !errors.Is(err, errRegistration) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errRegistration)
	}
	stoppedUnderTheStopGrace(t, err, &stopped)
}

func TestRunStopsThePluginsWhenARoleIsRefused(t *testing.T) {
	t.Parallel()

	var stopped atomic.Pointer[stopSeen]
	refused := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{
			stoppingPlugin{stopped: &stopped},
			rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"steward.manage_reports"}}}},
		}, nil
	}

	err := run(t.Context(), stopGraceEnv(t), io.Discard, refused)

	if !errors.Is(err, goncierge.ErrInvalidName) {
		t.Fatalf("run() error = %v, want %v in its chain", err, goncierge.ErrInvalidName)
	}
	stoppedUnderTheStopGrace(t, err, &stopped)
}

func TestRunStopsThePluginsWhenTheMailCannotBeBuilt(t *testing.T) {
	t.Parallel()

	var stopped atomic.Pointer[stopSeen]
	standing := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{stoppingPlugin{stopped: &stopped}}, nil
	}
	absentTemplateDir := filepath.Join(t.TempDir(), "absent")

	err := run(t.Context(), testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL":        testDatabaseURL(t),
		"ALPHONE_SHUTDOWN_STOP_GRACE": shutdownStopGrace.String(),
		"ALPHONE_SMTP_HOST":           "mail.example.com",
		"ALPHONE_SMTP_FROM":           "crm@example.com",
		"ALPHONE_SMTP_TLS":            "none",
		"ALPHONE_PUBLIC_URL":          "https://crm.example.com",
		"ALPHONE_MAIL_TEMPLATE_DIR":   absentTemplateDir,
	}), io.Discard, standing)

	if err == nil || !strings.Contains(err.Error(), absentTemplateDir) {
		t.Fatalf("run() error = %v, want the unusable template directory %s named", err, absentTemplateDir)
	}
	stoppedUnderTheStopGrace(t, err, &stopped)
}

func TestDeclareRolesGrantsEveryDeclarationAPluginMakes(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registered := []sdk.Plugin{
		silentPlugin{},
		rolePlugin{declared: []sdk.RoleDeclaration{
			{Name: "steward", Capabilities: []string{role.ManageUsers, "steward.manage_reports"}},
			{Name: role.Admin.String(), Capabilities: []string{"steward.manage_reports"}},
		}},
	}

	if err := declareRoles(registry, registered); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil", err)
	}

	if !registry.Can("steward", "steward.manage_reports") {
		t.Error("Can(steward, steward.manage_reports) = false, want the declared role held")
	}
	if !registry.Can(role.Admin.String(), "steward.manage_reports") {
		t.Error("Can(admin, steward.manage_reports) = false, want the core role widened")
	}
	if got := registry.HoldersOf(role.ManageUsers); !slices.Equal(got, []string{"admin", "steward"}) {
		t.Errorf("HoldersOf(manage_users) = %v, want the declared role counted as cover", got)
	}
}

func TestDeclareRolesRefusesADeclarationWithNoName(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registered := []sdk.Plugin{
		rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"steward.manage_reports"}}}},
	}

	err := declareRoles(registry, registered)

	if !errors.Is(err, goncierge.ErrInvalidName) {
		t.Errorf("declareRoles() error = %v, want ErrInvalidName surfaced at wiring", err)
	}
}

func TestDeclareRolesRefusesANewCapabilityOutsideThePluginPrefix(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	registered := []sdk.Plugin{
		rolePlugin{declared: []sdk.RoleDeclaration{{Name: "steward", Capabilities: []string{"manage_reports"}}}},
	}

	err := declareRoles(registry, registered)

	if !errors.Is(err, goncierge.ErrOutsidePrefix) || !strings.Contains(err.Error(), "plugin steward") {
		t.Errorf("declareRoles() error = %v, want ErrOutsidePrefix naming plugin steward", err)
	}
	if registry.Known("steward") {
		t.Error("the registry knows steward, want the refused declaration left out")
	}
}

func TestDeclareRolesRefusesAPluginUnderTheCoreID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		plugin sdk.Plugin
	}{
		{"one declaring roles", corePlugin{rolePlugin{declared: []sdk.RoleDeclaration{
			{Name: role.Admin.String(), Capabilities: []string{role.ManageUsers}},
		}}}},
		{"one declaring no roles", silentCorePlugin{}},
		{"one naming a top role", naming(role.Source, "owner", ownerRoles)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := role.NewRegistry()

			err := declareRoles(registry, []sdk.Plugin{tt.plugin})

			if !errors.Is(err, errReservedID) || !strings.Contains(err.Error(), "plugin core") {
				t.Errorf("declareRoles() error = %v, want %v naming plugin core", err, errReservedID)
			}
			keepsOnlyTheCoreRoles(t, registry)
		})
	}
}

func TestDeclareRolesGivesTheTopRoleWhatAPluginListedBeforeItGivesAdmin(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()

	if err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), naming("owners", "owner", ownerRoles)}); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil", err)
	}
	if !registry.Can(role.Admin.String(), "reports.export_reports") {
		t.Error("Can(admin, reports.export_reports) = false, want the core role widened")
	}
	if !registry.Can("owner", "reports.export_reports") {
		t.Error("Can(owner, reports.export_reports) = false, want the top role to hold every plugin capability")
	}
	if !registry.Outranks("owner", role.Admin.String()) {
		t.Error("Outranks(owner, admin) = false, want the top role to hold every capability admin holds")
	}
}

func TestDeclareRolesWithNoTopRoleKeepsAPluginCapabilityOffEveryOtherRole(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	reports := declaring("reports", []sdk.RoleDeclaration{
		{Name: role.Admin.String(), Capabilities: []string{"reports.export_reports"}},
		{Name: "auditor", Capabilities: []string{"reports.view_reports"}},
	})

	if err := declareRoles(registry, []sdk.Plugin{declaring("owners", ownerRoles), reports}); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil", err)
	}
	if got := registry.HoldersOf("reports.export_reports"); !slices.Equal(got, []string{"admin"}) {
		t.Errorf("HoldersOf(reports.export_reports) = %v, want admin alone when no plugin names a top role", got)
	}
	if got := registry.HoldersOf("reports.view_reports"); !slices.Equal(got, []string{"auditor"}) {
		t.Errorf("HoldersOf(reports.view_reports) = %v, want auditor alone when no plugin names a top role", got)
	}
}

func TestDeclareRolesGivesTheCapabilitiesToTheTopRoleItChecked(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	flipping := flippingTopRolePlugin{
		idRolePlugin: declaring("owners", ownerRoles), first: "owner", later: role.Member.String(), calls: &atomic.Int32{},
	}

	if err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), flipping}); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil", err)
	}
	if got := registry.HoldersOf("reports.export_reports"); !slices.Equal(got, []string{"admin", "owner"}) {
		t.Errorf("HoldersOf(reports.export_reports) = %v, want admin and the checked top role owner alone", got)
	}
}

func TestDeclareRolesRefusesAnEarlierPluginCarryingANewCapabilityOfThePluginNamingTheTopRole(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	clerks := declaring("reports", []sdk.RoleDeclaration{
		{Name: "clerk", Capabilities: []string{"owners.manage_workspaces"}},
	})
	owners := naming("owners", "owner", []sdk.RoleDeclaration{
		{Name: "owner", Capabilities: []string{role.ManageUsers, role.ManageWebhooks, "owners.manage_workspaces"}},
	})

	err := declareRoles(registry, []sdk.Plugin{clerks, owners})

	if !errors.Is(err, goncierge.ErrOutsidePrefix) || !strings.Contains(err.Error(), "plugin reports") {
		t.Errorf("declareRoles() error = %v, want ErrOutsidePrefix naming plugin reports, declared in load order", err)
	}
}

func TestDeclareRolesLetsThePluginNamingTheTopRoleCarryACapabilityOfAnEarlierPlugin(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	owners := naming("owners", "owner", []sdk.RoleDeclaration{
		ownerRoles[0],
		{Name: "steward", Capabilities: []string{"reports.export_reports"}},
	})

	if err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), owners}); err != nil {
		t.Fatalf("declareRoles() error = %v, want nil, each plugin declared in load order", err)
	}
	if !registry.Can("steward", "reports.export_reports") {
		t.Error("Can(steward, reports.export_reports) = false, want the later plugin to carry the earlier one's capability")
	}
}

func TestDeclareRolesRefusesATopRoleLackingACapabilityAdminCarries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		owners  []sdk.RoleDeclaration
		lacking string
	}{
		{"a capability AlphOne ships",
			[]sdk.RoleDeclaration{{Name: "owner", Capabilities: []string{role.ManageUsers}}}, role.ManageWebhooks},
		{"a capability its own plugin gives admin", []sdk.RoleDeclaration{
			ownerRoles[0], {Name: role.Admin.String(), Capabilities: []string{"owners.audit"}},
		}, "owners.audit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := role.NewRegistry()

			err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), naming("owners", "owner", tt.owners)})

			want := "plugin owners: the top role lacks a capability admin carries: " + tt.lacking
			if !errors.Is(err, errTopRoleLacksAdminCapability) || err.Error() != want {
				t.Errorf("declareRoles() error = %v, want %q", err, want)
			}
		})
	}
}

func TestDeclareRolesRefusesATopRoleThatALaterPluginUnderTheSameIDDrops(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	dropping := declaring("owners", []sdk.RoleDeclaration{{Name: "steward"}})

	err := declareRoles(registry, []sdk.Plugin{naming("owners", "owner", ownerRoles), reportsPlugin(), dropping})

	if want := `plugin reports: goncierge: unknown admin role: "owner"`; !errors.Is(err, goncierge.ErrUnknownAdmin) ||
		err.Error() != want {
		t.Errorf("declareRoles() error = %v, want %q", err, want)
	}
}

func TestDeclareRolesRefusesASecondPluginNamingATopRole(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	chiefs := naming("chiefs", "chief", []sdk.RoleDeclaration{{Name: "chief"}})

	err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), naming("owners", "owner", ownerRoles), chiefs})

	if !errors.Is(err, errSecondTopRole) || !strings.Contains(err.Error(), "plugins owners and chiefs") {
		t.Errorf("declareRoles() error = %v, want %v naming plugins owners and chiefs", err, errSecondTopRole)
	}
	keepsOnlyTheCoreRoles(t, registry)
}

func TestDeclareRolesRefusesATopRoleOtherThanANewRoleOfThePlugin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		plugin sdk.Plugin
		want   error
		named  string
	}{
		{"a role another plugin declares", naming("owners", "owner", []sdk.RoleDeclaration{{Name: "steward"}}),
			errUndeclaredTopRole, `plugin owners: the plugin does not declare its top role "owner"`},
		{"an empty role", naming("owners", "", ownerRoles), errEmptyTopRole, "plugin owners: the top role is empty"},
		{"a role while declaring none", silentTopRolePlugin{}, errUndeclaredTopRole,
			`plugin silent: the plugin does not declare its top role "owner"`},
		{"the member role AlphOne ships", naming("owners", role.Member.String(), []sdk.RoleDeclaration{
			{Name: role.Member.String()},
		}), errShippedTopRole, `plugin owners: AlphOne ships the top role "member"`},
		{"the admin role AlphOne ships", naming("owners", role.Admin.String(), []sdk.RoleDeclaration{
			{Name: role.Admin.String(), Capabilities: []string{role.ManageUsers, role.ManageWebhooks}},
		}), errShippedTopRole, `plugin owners: AlphOne ships the top role "admin"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registry := role.NewRegistry()

			err := declareRoles(registry, []sdk.Plugin{reportsPlugin(), declaring("staff", ownerRoles), tt.plugin})

			if !errors.Is(err, tt.want) || err.Error() != tt.named {
				t.Errorf("declareRoles() error = %v, want %v reading %q", err, tt.want, tt.named)
			}
			keepsOnlyTheCoreRoles(t, registry)
		})
	}
}
