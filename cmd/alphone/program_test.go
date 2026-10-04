// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/version"
	"github.com/gopherium/alphone/sdk"
)

// listedCommands are the names the listing of the command line holds, in the order it prints them.
var listedCommands = []string{"check", "help", "list", "migrate", "seed", "serve", "version"}

// bareProgram returns the command line over env, its plugins registering nothing into a role registry of its own.
func bareProgram(env map[string]string) gonsole.Program {
	return programOver(role.NewRegistry(), testGetenv(env), registeringNothing)
}

func TestProgramListsEveryCommandWhenNoneIsNamed(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "")

	want := strings.Join([]string{
		"AlphOne Version " + version.Version(),
		"",
		"Usage:",
		"  alphone <command> [flags] [arguments]",
		"",
		"Every command answers -h. A command that offers -json answers one JSON document. " +
			"A command that offers -yes is a dry run until -yes.",
		"",
		"Available commands:",
		"  check    check every setting, every plugin and every command name",
		"  help     print the help of one command",
		"  list     list every command",
		"  migrate  apply every schema step",
		"  seed     store the demo data",
		"  serve    run the server",
		"  version  print the version",
		"",
		"Every command is described at https://docs.alph.one/self-hosting/commands/",
		"",
	}, "\n")
	if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
		t.Errorf("bare run = %d, stdout %q, stderr %q, want 0 and the listing\n%s", got.Code, got.Stdout, got.Stderr, want)
	}
}

func TestProgramListsTheCommandsWhenAskedForHelp(t *testing.T) {
	t.Parallel()

	listing := testkit.Run(t, bareProgram(nil), "").Stdout
	for _, request := range []string{"help", "-h", "--help"} {
		t.Run(request, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(nil), "", request)

			if got.Code != gonsole.ExitDone || got.Stdout != listing || got.Stderr != "" {
				t.Errorf("%s = %d, stdout %q, stderr %q, want 0 and the listing", request, got.Code, got.Stdout, got.Stderr)
			}
		})
	}
}

func TestProgramRefusesAnUnknownCommand(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "not-a-command")

	want := "alphone: unknown command \"not-a-command\", run \"alphone list\" to see every command\n"
	if got.Code != gonsole.ExitMisused || got.Stdout != "" || got.Stderr != want {
		t.Errorf("not-a-command = %d, stdout %q, stderr %q, want 2 and %q", got.Code, got.Stdout, got.Stderr, want)
	}
}

func TestProgramAnswersEveryHelpPageWithoutADatabase(t *testing.T) {
	t.Parallel()

	for _, name := range listedCommands {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(nil), "", "help", name)

			if got.Code != gonsole.ExitDone || !strings.Contains(got.Stdout, "alphone "+name) || got.Stderr != "" {
				t.Errorf("help %s = %d, stdout %q, stderr %q, want 0 and its page", name, got.Code, got.Stdout, got.Stderr)
			}
		})
	}
}

func TestProgramServeNamesTheMissingDatabase(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "serve")

	if got.Code != gonsole.ExitFailed || got.Stderr != "alphone: ALPHONE_DATABASE_URL is required\n" {
		t.Errorf("serve = %d with stderr %q, want 1 and the missing database named", got.Code, got.Stderr)
	}
}

func TestProgramSeedOnlyPreviewsUntilYes(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)
	env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}
	for _, args := range [][]string{{"seed"}, {"seed", "--"}} {
		got := testkit.Run(t, bareProgram(env), "", args...)

		if got.Code != gonsole.ExitDone || got.Stdout != "would store the demo data\n" ||
			!strings.Contains(got.Stderr, "dry run, nothing changed, pass -yes to apply") {
			t.Errorf("%q = %d, stdout %q, stderr %q, want 0 and a preview", args, got.Code, got.Stdout, got.Stderr)
		}
		if schemas := extraSchemas(t, databaseURL); len(schemas) > 0 {
			t.Errorf("the database holds the schemas %v after %q, want nothing written", schemas, args)
		}
	}

	got := testkit.Run(t, bareProgram(env), "", "seed", "-yes")

	if got.Code != gonsole.ExitDone || !strings.Contains(got.Stdout, "seeded demo data") ||
		!strings.Contains(got.Stderr, "demo data is for development only") {
		t.Errorf("seed -yes = %d, stdout %q, stderr %q, want 0 and the demo data stored", got.Code, got.Stdout, got.Stderr)
	}
	if contacts := countRows(t, testPool(t, databaseURL), "core.contacts"); contacts == 0 {
		t.Error("the database holds no contact after seed -yes, want the demo contacts stored")
	}
}

func TestProgramRefusesArgumentsToSeedBeforeReadingTheDatabase(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
		want string
	}{
		"an argument":             {[]string{"seed", "now"}, "alphone: seed takes no arguments, got 1\n"},
		"a flag it does not know": {[]string{"seed", "-now"}, "alphone: seed: flag provided but not defined: -now\n"},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(nil), "", tc.args...)

			if got.Code != gonsole.ExitMisused || !strings.HasPrefix(got.Stderr, tc.want) {
				t.Errorf("%q = %d with stderr %q, want 2 and %q before the database is read",
					tc.args, got.Code, got.Stderr, tc.want)
			}
		})
	}
}

func TestRegisterPluginsCommands(t *testing.T) {
	t.Parallel()

	loaded, err := loadPlugins(role.NewRegistry(), registerPlugins)(t.Context(), describingCall(nil, io.Discard))

	if err != nil || loaded.Failed != nil {
		t.Fatalf("loadPlugins() = %v with %v failed, want every compiled plugin registered", err, loaded.Failed)
	}
	releaseAtEnd(t, loaded)
	if err := program(testGetenv(nil), registerPlugins).Check(loaded); err != nil {
		t.Errorf("Check() = %v, want every command a compiled plugin offers within the naming rules", err)
	}
}

func TestACommandStopsThePluginsItRegistered(t *testing.T) {
	t.Parallel()

	var stopped atomic.Pointer[stopSeen]
	env := map[string]string{
		"ALPHONE_DATABASE_URL":        unreachableDatabaseURL,
		"ALPHONE_SHUTDOWN_STOP_GRACE": shutdownStopGrace.String(),
	}
	stopping := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{stoppingPlugin{stopped: &stopped}}, nil
	}

	got := testkit.Run(t, programOver(role.NewRegistry(), testGetenv(env), stopping), "", "check")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, errStopFailed.Error()) {
		t.Errorf("check = %d with stderr %q, want 1 and the failed stop named", got.Code, got.Stderr)
	}
	seen := stopped.Load()
	if seen == nil {
		t.Fatal("the plugin was left running, want check to stop the plugins it registered")
	}
	if !underTheStopGrace(*seen) {
		t.Errorf("the plugin stopped with live = %v and %v left, want a live context holding most of the %v stop grace",
			seen.live, seen.left, shutdownStopGrace)
	}
}
