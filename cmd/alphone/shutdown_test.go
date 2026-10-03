// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/sdk"
)

// heldPath is the public route on which the holding plugin holds every request until it is cancelled.
const heldPath = "/held"

// slowShutdownGrace is the shutdown grace a held request outlasts.
const slowShutdownGrace = 50 * time.Millisecond

// shutdownStopGrace is the stop grace the shutdown tests set.
const shutdownStopGrace = 4 * time.Second

// freshStopWithin is how soon after the signal the plugins must be stopping under a live context.
const freshStopWithin = 2 * time.Second

// shutdownBudget bounds every wait of a shutdown test.
const shutdownBudget = 30 * time.Second

// stopGraceSlack is how much of the stop grace may be gone by the time a plugin's stop begins.
const stopGraceSlack = time.Second

// namedServing holds a distinct value for each serving setting, none of them a default.
var namedServing = gonsole.Timeouts{
	ReadHeader: 4 * time.Second, Read: 40 * time.Second, Idle: 90 * time.Second,
	Grace: 4 * time.Minute, CancelGrace: 15 * time.Second, StopGrace: 20 * time.Second,
}

// stopSeen is when the host stopped a plugin and what its context held then.
type stopSeen struct {
	at   time.Time
	live bool
	left time.Duration
}

// seenBy returns what a stop under ctx sees now.
func seenBy(ctx context.Context) stopSeen {
	seen := stopSeen{at: time.Now(), live: ctx.Err() == nil}
	if deadline, bounded := ctx.Deadline(); bounded {
		seen.left = time.Until(deadline)
	}
	return seen
}

// underTheStopGrace reports whether a stop saw a live context holding most of the stop grace the tests set.
func underTheStopGrace(seen stopSeen) bool {
	return seen.live && seen.left > shutdownStopGrace-stopGraceSlack && seen.left <= shutdownStopGrace
}

// holdingPlugin holds every request on its route until the request is cancelled, and reports what its stop saw.
type holdingPlugin struct {
	entered   chan struct{}
	enterOnce sync.Once
	stopped   chan stopSeen
}

// newHoldingPlugin returns a plugin ready to hold one request.
func newHoldingPlugin() *holdingPlugin {
	return &holdingPlugin{entered: make(chan struct{}), stopped: make(chan stopSeen, 1)}
}

// ID returns the plugin's identifier.
func (*holdingPlugin) ID() string {
	return "holding"
}

// Start starts nothing.
func (*holdingPlugin) Start(context.Context) error {
	return nil
}

// Stop reports when the host stopped the plugin and whether its context was live and how long it had left.
func (p *holdingPlugin) Stop(ctx context.Context) error {
	p.stopped <- seenBy(ctx)
	return nil
}

// PublicPaths opens the held route to callers without a session.
func (*holdingPlugin) PublicPaths() []string {
	return []string{heldPath}
}

// Routes returns the handler holding each request until it is cancelled.
func (p *holdingPlugin) Routes() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.enterOnce.Do(func() { close(p.entered) })
		<-r.Context().Done()
		w.WriteHeader(http.StatusServiceUnavailable)
	})
}

// besideTheCompiledPlugins returns a registration of the compiled plugins with plugin registered after them.
func besideTheCompiledPlugins(plugin sdk.Plugin) func(sdk.Deps) ([]sdk.Plugin, error) {
	return func(deps sdk.Deps) ([]sdk.Plugin, error) {
		registered, err := registerPlugins(deps)
		return append(registered, plugin), err
	}
}

// holdRequest holds a request on a transport of its own until the test ends, returning once the plugin has it.
func holdRequest(t *testing.T, addr string, plugin *holdingPlugin) {
	t.Helper()
	request, err := http.NewRequestWithContext(
		t.Context(), http.MethodGet, "http://"+addr+"/api/plugins/holding"+heldPath, nil)
	if err != nil {
		t.Fatalf("building the held request: %v", err)
	}
	transport := &http.Transport{}
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		if response, err := (&http.Client{Transport: transport}).Do(request); err == nil {
			_ = response.Body.Close()
		}
	}()
	t.Cleanup(func() {
		<-answered
		transport.CloseIdleConnections()
	})
	select {
	case <-plugin.entered:
	case <-time.After(shutdownBudget):
		t.Fatal("the held request never reached the plugin")
	}
}

// awaitStop returns what the holding plugin saw when the host stopped it, failing when it never stops.
func awaitStop(t *testing.T, plugin *holdingPlugin) stopSeen {
	t.Helper()
	select {
	case seen := <-plugin.stopped:
		return seen
	case <-time.After(shutdownBudget):
		t.Fatal("the plugin never stopped, want it stopped once the server shut down")
		return stopSeen{}
	}
}

// awaitRun returns what run answered, failing when it never returns.
func awaitRun(t *testing.T, finished <-chan error) error {
	t.Helper()
	select {
	case err := <-finished:
		return err
	case <-time.After(shutdownBudget):
		t.Fatal("run() never returned after the signal")
		return nil
	}
}

func TestRunNamesAMalformedShutdownGrace(t *testing.T) {
	t.Parallel()

	err := run(t.Context(), testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":   unreachableDatabaseURL,
		"ALPHONE_SHUTDOWN_GRACE": "soon",
	}), io.Discard, registerPlugins)

	want := `ALPHONE_SHUTDOWN_GRACE: must be a duration like 30s, got "soon"`
	if err == nil || err.Error() != want {
		t.Fatalf("run() error = %v, want %q before any database is reached", err, want)
	}
}

func TestRunGivesThePluginsAFreshStopGraceAfterASlowShutdown(t *testing.T) {
	t.Parallel()

	plugin := newHoldingPlugin()
	addr := freeAddr(t)
	env := testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":        testDatabaseURL(t),
		"ALPHONE_ADDR":                addr,
		"ALPHONE_SHUTDOWN_GRACE":      slowShutdownGrace.String(),
		"ALPHONE_SHUTDOWN_STOP_GRACE": shutdownStopGrace.String(),
	})
	ctx, signal := context.WithCancel(t.Context())
	defer signal()
	finished := make(chan error, 1)
	go func() { finished <- run(ctx, env, io.Discard, besideTheCompiledPlugins(plugin)) }()
	waitForServer(t, "http://"+addr)
	holdRequest(t, addr, plugin)

	signalled := time.Now()
	signal()

	seen := awaitStop(t, plugin)
	waited := seen.at.Sub(signalled)
	if waited > freshStopWithin || !underTheStopGrace(seen) {
		t.Errorf("the plugins stopped %v after the signal with live = %v and %v left, "+
			"want a live context holding most of the %v stop grace within %v", waited, seen.live, seen.left,
			shutdownStopGrace, freshStopWithin)
	}
	if err := awaitRun(t, finished); err != nil {
		t.Errorf("run() error = %v, want a normal stop once the held request was cancelled", err)
	}
}

func TestTheServingSettingsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": unreachableDatabaseURL,
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	want := gonsole.Timeouts{
		ReadHeader: 10 * time.Second, Read: 30 * time.Second, Idle: 2 * time.Minute,
		Grace: 10 * time.Second, CancelGrace: 5 * time.Second, StopGrace: 5 * time.Second,
	}
	if held.serving != want {
		t.Errorf("serving = %+v, want the defaults %+v", held.serving, want)
	}
}

func TestTheServingSettingsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":             unreachableDatabaseURL,
		"ALPHONE_HTTP_READ_HEADER_TIMEOUT": "4s",
		"ALPHONE_HTTP_READ_TIMEOUT":        "40s",
		"ALPHONE_HTTP_IDLE_TIMEOUT":        "90s",
		"ALPHONE_SHUTDOWN_GRACE":           "4m",
		"ALPHONE_SHUTDOWN_CANCEL_GRACE":    "15s",
		"ALPHONE_SHUTDOWN_STOP_GRACE":      "20s",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.serving != namedServing {
		t.Errorf("serving = %+v, want each value the environment named %+v", held.serving, namedServing)
	}
}

func TestTheHTTPServerCarriesTheTimeoutsTheSettingsName(t *testing.T) {
	t.Parallel()

	settings := runConfig{addr: "localhost:9999", serving: namedServing}

	held := httpServerFrom(settings, http.NotFoundHandler())

	for name, asked := range map[string]struct {
		stood time.Duration
		want  time.Duration
	}{
		"ReadHeaderTimeout": {held.ReadHeaderTimeout, namedServing.ReadHeader},
		"ReadTimeout":       {held.ReadTimeout, namedServing.Read},
		"IdleTimeout":       {held.IdleTimeout, namedServing.Idle},
	} {
		if asked.stood != asked.want {
			t.Errorf("%s = %v, want %v", name, asked.stood, asked.want)
		}
	}
	if held.Addr != settings.addr {
		t.Errorf("Addr = %q, want %q", held.Addr, settings.addr)
	}
	if held.Handler == nil {
		t.Error("Handler = nil, want the handler served")
	}
}
