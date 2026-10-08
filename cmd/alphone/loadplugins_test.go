// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// releaseStopGrace is the stop grace a release that holds must give up within.
const releaseStopGrace = 50 * time.Millisecond

// blockingStopPlugin holds its stop until the host's context ends.
type blockingStopPlugin struct{ inertPlugin }

// Stop waits for ctx to end and answers why it ended.
func (blockingStopPlugin) Stop(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

// commandingPlugin offers one command under its id.
type commandingPlugin struct{ inertPlugin }

// Commands returns the plugin's one command.
func (commandingPlugin) Commands() []sdk.Command {
	return []sdk.Command{{
		Name: "probe:show", Summary: "show nothing", Run: func(context.Context, sdk.Call) error { return nil },
	}}
}

// describingCall returns a call that only describes commands, over env and writing its progress to stderr.
func describingCall(env map[string]string, stderr io.Writer) gonsole.Call {
	return gonsole.Call{Env: settingsEnv(testkit.Getenv(env)), Stderr: stderr, Describe: true}
}

// loadingProgram returns a program over env that registers its plugins through loadPlugins and nothing else.
func loadingProgram(env map[string]string, plugins func(sdk.Deps) ([]sdk.Plugin, error)) gonsole.Program {
	return gonsole.Program{
		Name:     "alphone",
		Env:      settingsEnv(testkit.Getenv(env)),
		Database: "DATABASE_URL",
		Plugins:  loadPlugins(role.NewRegistry(), plugins),
	}
}

// releaseAtEnd stops what a load registered once the test ends.
func releaseAtEnd(t *testing.T, loaded gonsole.Loaded) {
	t.Helper()
	t.Cleanup(func() {
		if err := loaded.Release(context.Background()); err != nil {
			t.Errorf("Release() error = %v, want nil", err)
		}
	})
}

func TestLoadPluginsInDescribeModeNeedsNoDatabaseSetting(t *testing.T) {
	t.Parallel()

	loaded, err := loadPlugins(role.NewRegistry(), registerPlugins)(t.Context(), describingCall(nil, io.Discard))

	if err != nil || loaded.Failed != nil {
		t.Fatalf("loadPlugins() = %v with %v failed, want every compiled plugin registered", err, loaded.Failed)
	}
	releaseAtEnd(t, loaded)
}

func TestLoadPluginsNamesWhatItCannotRead(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		env  map[string]string
		want string
	}{
		"no database address outside describe mode": {map[string]string{}, "ALPHONE_DATABASE_URL is required"},
		"a stop grace that is not a duration": {
			map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL, "ALPHONE_SHUTDOWN_STOP_GRACE": "soon"},
			`ALPHONE_SHUTDOWN_STOP_GRACE: must be a duration like 30s, got "soon"`,
		},
		"a stop grace of zero": {
			map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL, "ALPHONE_SHUTDOWN_STOP_GRACE": "0s"},
			`ALPHONE_SHUTDOWN_STOP_GRACE: must stand above zero, got "0s"`,
		},
		"a mail port in words": {
			map[string]string{
				"ALPHONE_DATABASE_URL": unreachableDatabaseURL,
				"ALPHONE_SMTP_HOST":    "mail.example.com",
				"ALPHONE_SMTP_PORT":    "the submission port",
			},
			`ALPHONE_SMTP_PORT: must be a whole number, got "the submission port"`,
		},
		"a webhook host without a port": {
			map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL, "ALPHONE_WEBHOOK_ALLOWED_HOSTS": "n8n"},
			"ALPHONE_WEBHOOK_ALLOWED_HOSTS: must list CIDR ranges such as 127.0.0.1/32 or host names with a port",
		},
		"a database address it cannot parse": {
			map[string]string{"ALPHONE_DATABASE_URL": "://not-a-url"}, "parse database url",
		},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, loadingProgram(tc.env, registerPlugins), "", "check")

			if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, tc.want) {
				t.Errorf("check = %d with stderr %q, want 1 and %q", got.Code, got.Stderr, tc.want)
			}
		})
	}
}

func TestReleaseEndsWithinTheStopGrace(t *testing.T) {
	t.Parallel()

	call := describingCall(map[string]string{"ALPHONE_SHUTDOWN_STOP_GRACE": releaseStopGrace.String()}, io.Discard)
	loaded, err := loadPlugins(role.NewRegistry(), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{blockingStopPlugin{inertPlugin{id: "blocking"}}}, nil
	})(t.Context(), call)
	if err != nil {
		t.Fatalf("loadPlugins() error = %v, want nil", err)
	}
	started := time.Now()

	err = loaded.Release(t.Context())

	if took := time.Since(started); !errors.Is(err, context.DeadlineExceeded) || took > stopGraceSlack {
		t.Errorf("Release() = %v after %v, want it to give up once the %v stop grace ran out",
			err, took, releaseStopGrace)
	}
}

func TestLoadPluginsStopsThePluginsWhenARoleIsRefused(t *testing.T) {
	t.Parallel()

	var stopped atomic.Pointer[stopSeen]
	call := describingCall(map[string]string{"ALPHONE_SHUTDOWN_STOP_GRACE": shutdownStopGrace.String()}, io.Discard)

	_, err := loadPlugins(role.NewRegistry(), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{
			stoppingPlugin{stopped: &stopped},
			rolePlugin{declared: []sdk.RoleDeclaration{{Name: "", Capabilities: []string{"manage_reports"}}}},
		}, nil
	})(t.Context(), call)

	if !errors.Is(err, role.ErrEmptyRole) {
		t.Fatalf("loadPlugins() error = %v, want %v in its chain", err, role.ErrEmptyRole)
	}
	stoppedUnderTheStopGrace(t, err, &stopped)
}

func TestLoadPluginsKeepsInfoLinesOffStderrAndShowsErrors(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder

	loaded, err := loadPlugins(role.NewRegistry(), func(deps sdk.Deps) ([]sdk.Plugin, error) {
		deps.Events.Publish(t.Context(), "no.such.event", nil)
		return nil, nil
	})(t.Context(), describingCall(nil, &stderr))

	if err != nil {
		t.Fatalf("loadPlugins() error = %v, want nil", err)
	}
	releaseAtEnd(t, loaded)
	if strings.Contains(stderr.String(), "no mail relay configured") {
		t.Errorf("stderr = %q, want the info lines of compose kept off a command's stderr", stderr.String())
	}
	if !strings.Contains(stderr.String(), "refusing to publish") {
		t.Errorf("stderr = %q, want the refused publish shown as an error", stderr.String())
	}
}

func TestLoadPluginsCarriesTheCommandGroupsAndTheRegistrationFailure(t *testing.T) {
	t.Parallel()

	loaded, err := loadPlugins(role.NewRegistry(), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{commandingPlugin{inertPlugin{id: "probe"}}}, errRegistration
	})(t.Context(), describingCall(nil, io.Discard))

	if err != nil {
		t.Fatalf("loadPlugins() error = %v, want nil", err)
	}
	releaseAtEnd(t, loaded)
	if !errors.Is(loaded.Failed, errRegistration) {
		t.Errorf("Failed = %v, want the registration failure", loaded.Failed)
	}
	if len(loaded.Groups) != 1 || loaded.Groups[0].Namespace != "probe" {
		t.Errorf("Groups = %v, want the one group the probe plugin offers", loaded.Groups)
	}
}

func TestReleaseClosesThePoolComposeOpened(t *testing.T) {
	t.Parallel()

	var contacts sdk.ContactDirectory
	call := describingCall(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}, io.Discard)
	loaded, err := loadPlugins(role.NewRegistry(), func(deps sdk.Deps) ([]sdk.Plugin, error) {
		contacts = deps.Contacts
		return nil, nil
	})(t.Context(), call)
	if err != nil {
		t.Fatalf("loadPlugins() error = %v, want nil", err)
	}
	if err := loaded.Release(t.Context()); err != nil {
		t.Fatalf("Release() error = %v, want nil", err)
	}

	_, _, err = contacts.FindByIdentity(t.Context(), "email", "maria.perez@example.com")

	if err == nil || !strings.Contains(err.Error(), "closed pool") {
		t.Errorf("FindByIdentity() after Release error = %v, want the closed pool refused", err)
	}
}

func TestLoadPluginsNamesTheRegistrationFailureBesideARefusedRole(t *testing.T) {
	t.Parallel()

	_, err := loadPlugins(role.NewRegistry(), partlyRegisteredWithARefusedRole)(
		t.Context(), describingCall(nil, io.Discard))

	if !errors.Is(err, errRegistration) || !errors.Is(err, role.ErrEmptyRole) {
		t.Errorf("loadPlugins() error = %v, want the registration failure and the refused role both named", err)
	}
}
