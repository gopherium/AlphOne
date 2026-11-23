// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/framework/pluginkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/sdk"
)

// hiddenPluginsDocument reads the ids of the plugins that hide their screens in the caller's workspace.
const hiddenPluginsDocument = `{ hiddenPlugins }`

// screensBudget is the time one ask of the screens hook gives its plugins in a test that waits for none.
const screensBudget = time.Minute

// lateBudget is the time one ask of the screens hook gives a plugin that answers only once that time ran out.
const lateBudget = time.Millisecond

// errMailboxUnreadable is the failure of a plugin that cannot read what its screens show.
var errMailboxUnreadable = errors.New("mailbox unreadable")

// hiddenPluginsResult is the shape the hidden plugins read answers.
type hiddenPluginsResult struct {
	HiddenPlugins []string `json:"hiddenPlugins"`
}

// screensPlugin is a plugin of a test that answers whether its screens show.
type screensPlugin struct {
	id     string
	hides  bool
	err    error
	panics string
	late   bool
}

// ID reports the plugin identifier.
func (p screensPlugin) ID() string { return p.id }

// Start does nothing.
func (p screensPlugin) Start(context.Context) error { return nil }

// Stop does nothing.
func (p screensPlugin) Stop(context.Context) error { return nil }

// ShowsScreens answers that the screens show unless the test hides them, after ctx ended when the plugin is late.
func (p screensPlugin) ShowsScreens(ctx context.Context) (bool, error) {
	if p.panics != "" {
		panic(p.panics)
	}
	if p.late {
		<-ctx.Done()
	}
	return !p.hides, p.err
}

// bystanderPlugin is a plugin of a test that never says whether its screens show.
type bystanderPlugin struct{ id string }

// ID reports the plugin identifier.
func (p bystanderPlugin) ID() string { return p.id }

// Start does nothing.
func (p bystanderPlugin) Start(context.Context) error { return nil }

// Stop does nothing.
func (p bystanderPlugin) Stop(context.Context) error { return nil }

// askHidden returns the plugins hiding their screens through one screens hook under budget, logging to log.
func askHidden(t *testing.T, plugins []sdk.Plugin, budget time.Duration, log *bytes.Buffer) []string {
	t.Helper()
	hook, err := pluginkit.NewHook[graphres.ScreenGated](graphres.ScreensHook, plugins, pluginkit.HookSettings{
		Budget: budget, TimedFirings: 20, Report: graphres.LogHookFailure(slog.New(slog.NewTextHandler(log, nil))),
	})
	if err != nil {
		t.Fatalf("NewHook() error = %v, want nil", err)
	}
	return pluginkit.Filter(t.Context(), hook, []string{}, graphres.HideScreens)
}

// hookFailures returns the hook failure lines of log.
func hookFailures(log *bytes.Buffer) []string {
	return slices.DeleteFunc(strings.Split(log.String(), "\n"), func(line string) bool {
		return !strings.Contains(line, `msg="plugin hook failed"`)
	})
}

func TestHiddenPluginsAnswerTheIdsTheReadNamesForTheCallersWorkspace(t *testing.T) {
	t.Parallel()

	acme := uuid.Must(uuid.NewV7())
	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})
	resolver.HiddenScreens = func(ctx context.Context) []string {
		if sdk.TenantOrDefault(ctx) == acme {
			return []string{"email", "notes"}
		}
		return []string{}
	}
	caller := uuid.Must(uuid.NewV7())
	inAcme := newDecoratedGraphClient(t, resolver, func(ctx context.Context) context.Context {
		return sdk.WithTenant(authkitIdentity(ctx, caller), acme)
	})
	elsewhere := newGraphClient(t, resolver, caller)

	var acmeAnswer, elsewhereAnswer hiddenPluginsResult
	inAcme.MustPost(hiddenPluginsDocument, &acmeAnswer)
	elsewhere.MustPost(hiddenPluginsDocument, &elsewhereAnswer)

	if !slices.Equal(acmeAnswer.HiddenPlugins, []string{"email", "notes"}) {
		t.Errorf("hiddenPlugins in the workspace = %v, want [email notes] the read names there",
			acmeAnswer.HiddenPlugins)
	}
	if elsewhereAnswer.HiddenPlugins == nil || len(elsewhereAnswer.HiddenPlugins) != 0 {
		t.Errorf("hiddenPlugins in another workspace = %v, want an empty list", elsewhereAnswer.HiddenPlugins)
	}
}

func TestHiddenPluginsAnswerNoneWithoutARead(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, newStubResolver(&stubContactStore{}, &stubTaskStore{}), uuid.Must(uuid.NewV7()))

	var answer hiddenPluginsResult
	client.MustPost(hiddenPluginsDocument, &answer)

	if answer.HiddenPlugins == nil || len(answer.HiddenPlugins) != 0 {
		t.Errorf("hiddenPlugins = %v, want an empty list when nothing reads the plugins", answer.HiddenPlugins)
	}
}

func TestHiddenPluginsRejectACallerWhoIsNotSignedIn(t *testing.T) {
	t.Parallel()

	resolver := newStubResolver(&stubContactStore{}, &stubTaskStore{})
	resolver.HiddenScreens = func(context.Context) []string { return []string{"email"} }
	client := newAnonymousGraphClient(t, resolver)

	raw, err := client.RawPost(hiddenPluginsDocument)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want an answer", err)
	}
	if code := firstErrorCode(t, raw.Errors); code != "UNAUTHENTICATED" {
		t.Errorf("code = %q, want UNAUTHENTICATED", code)
	}
	if raw.Data != nil {
		t.Errorf("data = %v, want none for a caller who is not signed in", raw.Data)
	}
}

func TestHideScreensListsThePluginsThatHideTheirScreensInOrder(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	plugins := []sdk.Plugin{
		screensPlugin{id: "notes", hides: true}, bystanderPlugin{id: "theme"},
		screensPlugin{id: "feed"}, screensPlugin{id: "email", hides: true},
	}

	hidden := askHidden(t, plugins, screensBudget, &log)

	if !slices.Equal(hidden, []string{"notes", "email"}) {
		t.Errorf("hidden = %v, want notes then email", hidden)
	}
	if failures := hookFailures(&log); len(failures) != 0 {
		t.Errorf("the log holds %q, want no failure", failures)
	}
}

func TestAFailingOrPanickingPluginKeepsItsScreensShownAndIsLoggedOnce(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	plugins := []sdk.Plugin{
		screensPlugin{id: "email", hides: true, err: errMailboxUnreadable},
		screensPlugin{id: "archive", hides: true},
		screensPlugin{id: "notes", hides: true, panics: "boom"},
	}

	hidden := askHidden(t, plugins, screensBudget, &log)

	if !slices.Equal(hidden, []string{"archive"}) {
		t.Errorf("hidden = %v, want archive alone, the failing and the panicking plugin shown", hidden)
	}
	failures := hookFailures(&log)
	if len(failures) != 2 {
		t.Fatalf("the log holds %q, want one line for each of the two failures", failures)
	}
	failed := `msg="plugin hook failed" plugin=email hook=admin.screens panicked=false context_ended=false ` +
		`error="pluginkit: email admin.screens: mailbox unreadable" stack=""`
	if !strings.Contains(failures[0], failed) {
		t.Errorf("the first failure logs %q, want %s", failures[0], failed)
	}
	panicked := `msg="plugin hook failed" plugin=notes hook=admin.screens panicked=true context_ended=false ` +
		`error="pluginkit: notes admin.screens panicked: boom" stack="goroutine `
	if !strings.Contains(failures[1], panicked) {
		t.Errorf("the second failure logs %q, want %s and the stack", failures[1], panicked)
	}
}

func TestAPluginThatAnswersAfterItsTimeRanOutKeepsItsScreensShown(t *testing.T) {
	t.Parallel()

	var log bytes.Buffer
	plugins := []sdk.Plugin{screensPlugin{id: "email", hides: true, late: true}}

	hidden := askHidden(t, plugins, lateBudget, &log)

	if len(hidden) != 0 {
		t.Errorf("hidden = %v, want none, the late plugin shown", hidden)
	}
	failures := hookFailures(&log)
	late := `msg="plugin hook failed" plugin=email hook=admin.screens panicked=false context_ended=true ` +
		`error="pluginkit: email admin.screens: context deadline exceeded" stack=""`
	if len(failures) != 1 || !strings.Contains(failures[0], late) {
		t.Errorf("the log holds %q, want the one line %s", failures, late)
	}
}
