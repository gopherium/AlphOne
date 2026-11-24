// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/sdk"
)

// errScreensUnread is the failure of a plugin that cannot read what its screens show.
var errScreensUnread = errors.New("screens unread")

// screensGatePlugin is a plugin of a test that answers whether its screens show.
type screensGatePlugin struct {
	inertPlugin
	hides bool
	err   error
}

// ShowsScreens answers that the screens show unless the test hides them, with the failure the test gives it.
func (p screensGatePlugin) ShowsScreens(context.Context) (bool, error) {
	return !p.hides, p.err
}

func TestTheHookSettingsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": "postgres://localhost/x"}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if want := (hookSettings{screensBudget: 50 * time.Millisecond, timedFirings: 20}); held.hooks != want {
		t.Errorf("hook settings = %+v, want the defaults %+v", held.hooks, want)
	}
}

func TestTheHookSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		budget  string
		firings string
		want    hookSettings
	}{
		"the shortest budget and one timed firing": {"1ns", "1", hookSettings{screensBudget: 1, timedFirings: 1}},
		"a longer budget and the most timed firings": {
			"250ms", "1000", hookSettings{screensBudget: 250 * time.Millisecond, timedFirings: 1000},
		},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			held, err := loadRunConfig(testkit.Getenv(map[string]string{
				"ALPHONE_DATABASE_URL":              "postgres://localhost/x",
				"ALPHONE_PLUGIN_SCREENS_BUDGET":     tt.budget,
				"ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS": tt.firings,
			}))

			if err != nil {
				t.Fatalf("loadRunConfig() error = %v, want nil", err)
			}
			if held.hooks != tt.want {
				t.Errorf("hook settings = %+v, want %+v", held.hooks, tt.want)
			}
		})
	}
}

func TestTheHookSettingsRejectAValueOutOfBounds(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		named  string
		value  string
		reason string
	}{
		"no screens budget":             {"ALPHONE_PLUGIN_SCREENS_BUDGET", "0s", "must stand above zero"},
		"a screens budget below zero":   {"ALPHONE_PLUGIN_SCREENS_BUDGET", "-5ms", "must stand above zero"},
		"a screens budget with no unit": {"ALPHONE_PLUGIN_SCREENS_BUDGET", "50", "must be a duration like 30s"},
		"no timed firing":               {"ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS", "0", "must stand above zero"},
		"one timed firing too many":     {"ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS", "1001", "must stand at or below 1000"},
		"timed firings in words":        {"ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS", "twenty", "must be a whole number"},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			want := fmt.Sprintf("%s: %s, got %q", tt.named, tt.reason, tt.value)

			_, err := loadRunConfig(testkit.Getenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				tt.named:               tt.value,
			}))

			if err == nil || err.Error() != want {
				t.Errorf("loadRunConfig() error = %v, want %q", err, want)
			}
		})
	}
}

func TestTheExampleFileStatesTheHookFallbacks(t *testing.T) {
	t.Parallel()

	named := exampleSettings(t)
	fallbacks := map[string]string{
		"ALPHONE_PLUGIN_SCREENS_BUDGET":     defaultScreensBudget.String(),
		"ALPHONE_PLUGIN_HOOK_TIMED_FIRINGS": strconv.Itoa(defaultHookTimedFirings),
	}

	for key, want := range fallbacks {
		if named[key] != want {
			t.Errorf("the example file states %s=%q, want the fallback %q", key, named[key], want)
		}
	}
}

func TestTheScreensReadNamesThePluginsThatHideTheirScreens(t *testing.T) {
	t.Parallel()

	registered := []sdk.Plugin{
		screensGatePlugin{inertPlugin: inertPlugin{id: "notes"}, hides: true}, inertPlugin{id: "theme"},
		screensGatePlugin{inertPlugin: inertPlugin{id: "feed"}},
		screensGatePlugin{inertPlugin: inertPlugin{id: "email"}, hides: true},
	}

	read := screensHidden(registered, hookSettings{screensBudget: time.Minute, timedFirings: 1}, discardLogger())

	if hidden := read(t.Context()); !slices.Equal(hidden, []string{"notes", "email"}) {
		t.Errorf("the screens read = %v, want notes then email", hidden)
	}
}

func TestTheScreensReadKeepsAFailingPluginShownAndLogsIt(t *testing.T) {
	t.Parallel()

	var logged lockedLog
	registered := []sdk.Plugin{
		screensGatePlugin{inertPlugin: inertPlugin{id: "email"}, hides: true, err: errScreensUnread},
		screensGatePlugin{inertPlugin: inertPlugin{id: "notes"}, hides: true},
	}
	read := screensHidden(registered, hookSettings{screensBudget: time.Minute, timedFirings: 1},
		slog.New(slog.NewTextHandler(&logged, nil)))

	hidden := read(t.Context())

	if !slices.Equal(hidden, []string{"notes"}) {
		t.Errorf("the screens read = %v, want notes alone, the failing plugin shown", hidden)
	}
	want := `msg="plugin hook failed" plugin=email hook=admin.screens panicked=false context_ended=false ` +
		`error="pluginkit: email admin.screens: screens unread"`
	if got := logged.String(); strings.Count(got, `msg="plugin hook failed"`) != 1 || !strings.Contains(got, want) {
		t.Errorf("the server log holds %q, want the one line %s", got, want)
	}
}

func TestTheScreensReadPanicsUnderSettingsNoHookRunsUnder(t *testing.T) {
	t.Parallel()

	defer func() {
		if recovered := fmt.Sprint(recover()); !strings.Contains(recovered, "the budget must stand above zero") {
			t.Errorf("screensHidden() panicked with %q, want the budget no hook runs under named", recovered)
		}
	}()

	screensHidden(nil, hookSettings{timedFirings: 1}, discardLogger())
}
