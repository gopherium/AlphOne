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
			rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"manage_reports"}}}},
		}, nil
	}

	err := run(t.Context(), stopGraceEnv(t), io.Discard, refused)

	if !errors.Is(err, role.ErrEmptyRole) {
		t.Fatalf("run() error = %v, want %v in its chain", err, role.ErrEmptyRole)
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
