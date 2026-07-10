// SPDX-License-Identifier: Elastic-2.0

package plugin_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/gopherium/alphone/internal/plugin"
	"github.com/gopherium/alphone/sdk"
)

var (
	_ sdk.Plugin             = (*fakePlugin)(nil)
	_ sdk.Migrator           = (*migratingPlugin)(nil)
	_ sdk.RouteProvider      = (*routedPlugin)(nil)
	_ sdk.PublicPathProvider = (*publicPathsPlugin)(nil)
)

type publicPathsPlugin struct {
	fakePlugin
	paths []string
}

func (p *publicPathsPlugin) PublicPaths() []string {
	return p.paths
}

func TestHostCollectsPublicPaths(t *testing.T) {
	t.Parallel()

	var calls []string
	host := plugin.NewHost(
		&publicPathsPlugin{fakePlugin: fakePlugin{id: "hooked", calls: &calls}, paths: []string{"/webhook"}},
		&fakePlugin{id: "plain", calls: &calls},
	)

	got := host.PublicPaths()

	want := map[string][]string{"hooked": {"/webhook"}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("PublicPaths() mismatch (-want +got):\n%s", diff)
	}
}

type routedPlugin struct {
	fakePlugin
	handler http.Handler
}

func (r *routedPlugin) Routes() http.Handler {
	return r.handler
}

type fakePlugin struct {
	id         string
	startErr   error
	stopErr    error
	startPanic bool
	stopPanic  bool
	calls      *[]string
}

func (f *fakePlugin) ID() string {
	return f.id
}

func (f *fakePlugin) Start(_ context.Context) error {
	*f.calls = append(*f.calls, f.id+" start")
	if f.startPanic {
		panic("boom")
	}
	return f.startErr
}

func (f *fakePlugin) Stop(_ context.Context) error {
	*f.calls = append(*f.calls, f.id+" stop")
	if f.stopPanic {
		panic("boom")
	}
	return f.stopErr
}

type migratingPlugin struct {
	fakePlugin
	migrateErr   error
	migratePanic bool
}

func (m *migratingPlugin) Migrate(_ context.Context) error {
	*m.calls = append(*m.calls, m.id+" migrate")
	if m.migratePanic {
		panic("boom")
	}
	return m.migrateErr
}

func TestHostStartsInOrderAndStopsInReverse(t *testing.T) {
	t.Parallel()

	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", calls: &calls},
	)

	if err := host.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}
	if err := host.Stop(t.Context()); err != nil {
		t.Fatalf("Stop() error = %v, want nil", err)
	}

	want := []string{"alpha start", "beta start", "beta stop", "alpha stop"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("lifecycle calls mismatch (-want +got):\n%s", diff)
	}
}

func TestNewHostPanicsOnDuplicateIDs(t *testing.T) {
	t.Parallel()

	defer func() {
		if recover() == nil {
			t.Fatal("NewHost() did not panic, want a duplicate id panic")
		}
	}()

	var calls []string
	plugin.NewHost(
		&fakePlugin{id: "whatsapp", calls: &calls},
		&fakePlugin{id: "whatsapp", calls: &calls},
	)
}

func TestHostCollectsRoutesFromProviders(t *testing.T) {
	t.Parallel()

	var calls []string
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&routedPlugin{fakePlugin: fakePlugin{id: "beta", calls: &calls}, handler: handler},
	)

	routes := host.Routes()

	if len(routes) != 1 {
		t.Fatalf("Routes() returned %d entries, want 1", len(routes))
	}
	if _, ok := routes["beta"]; !ok {
		t.Error(`Routes() has no entry for "beta", want its handler`)
	}
}

func TestHostMigratesBeforeStarting(t *testing.T) {
	t.Parallel()

	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&migratingPlugin{fakePlugin: fakePlugin{id: "beta", calls: &calls}},
	)

	if err := host.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	want := []string{"beta migrate", "alpha start", "beta start"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("migration ordering mismatch (-want +got):\n%s", diff)
	}
}

func TestHostAbortsWhenMigrationFails(t *testing.T) {
	t.Parallel()

	errSchema := errors.New("schema exploded")
	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&migratingPlugin{fakePlugin: fakePlugin{id: "beta", calls: &calls}, migrateErr: errSchema},
	)

	startErr := host.Start(t.Context())

	if !errors.Is(startErr, errSchema) {
		t.Fatalf("Start() error = %v, want %v in its chain", startErr, errSchema)
	}
	want := []string{"beta migrate"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("abort calls mismatch (-want +got):\n%s", diff)
	}
}

func TestHostRecoversMigrationPanic(t *testing.T) {
	t.Parallel()

	var calls []string
	host := plugin.NewHost(
		&migratingPlugin{fakePlugin: fakePlugin{id: "beta", calls: &calls}, migratePanic: true},
	)

	if err := host.Start(t.Context()); err == nil {
		t.Fatal("Start() error = nil, want a recovered panic error")
	}
}

func TestHostRollsBackWhenStartFails(t *testing.T) {
	t.Parallel()

	errBoot := errors.New("boot failed")
	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", startErr: errBoot, calls: &calls},
		&fakePlugin{id: "gamma", calls: &calls},
	)

	startErr := host.Start(t.Context())

	if !errors.Is(startErr, errBoot) {
		t.Fatalf("Start() error = %v, want %v in its chain", startErr, errBoot)
	}
	want := []string{"alpha start", "beta start", "alpha stop"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("rollback calls mismatch (-want +got):\n%s", diff)
	}
}

func TestHostRecoversStartPanic(t *testing.T) {
	t.Parallel()

	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", startPanic: true, calls: &calls},
	)

	startErr := host.Start(t.Context())

	if startErr == nil {
		t.Fatal("Start() error = nil, want a recovered panic error")
	}
	want := []string{"alpha start", "beta start", "alpha stop"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("rollback calls mismatch (-want +got):\n%s", diff)
	}
}

func TestHostStopCollectsAllFailures(t *testing.T) {
	t.Parallel()

	errAlpha := errors.New("alpha refused")
	var calls []string
	host := plugin.NewHost(
		&fakePlugin{id: "alpha", stopErr: errAlpha, calls: &calls},
		&fakePlugin{id: "beta", stopPanic: true, calls: &calls},
	)
	if err := host.Start(t.Context()); err != nil {
		t.Fatalf("Start() error = %v, want nil", err)
	}

	stopErr := host.Stop(t.Context())

	if !errors.Is(stopErr, errAlpha) {
		t.Fatalf("Stop() error = %v, want %v in its chain", stopErr, errAlpha)
	}
	want := []string{"alpha start", "beta start", "beta stop", "alpha stop"}
	if diff := cmp.Diff(want, calls); diff != "" {
		t.Errorf("stop calls mismatch (-want +got):\n%s", diff)
	}
}
