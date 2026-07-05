// SPDX-License-Identifier: AGPL-3.0-or-later

package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/gopherium/alphone/internal/plugin"
)

var _ plugin.Plugin = (*fakePlugin)(nil)

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

func TestHostStartsInOrderAndStopsInReverse(t *testing.T) {
	t.Parallel()

	var calls []string
	host, err := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", calls: &calls},
	)
	if err != nil {
		t.Fatalf("NewHost() error = %v, want nil", err)
	}

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

func TestNewHostRejectsDuplicateIDs(t *testing.T) {
	t.Parallel()

	var calls []string

	_, err := plugin.NewHost(
		&fakePlugin{id: "whatsapp", calls: &calls},
		&fakePlugin{id: "whatsapp", calls: &calls},
	)

	if err == nil {
		t.Fatal("NewHost() error = nil, want a duplicate id error")
	}
}

func TestHostRollsBackWhenStartFails(t *testing.T) {
	t.Parallel()

	errBoot := errors.New("boot failed")
	var calls []string
	host, err := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", startErr: errBoot, calls: &calls},
		&fakePlugin{id: "gamma", calls: &calls},
	)
	if err != nil {
		t.Fatalf("NewHost() error = %v, want nil", err)
	}

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
	host, err := plugin.NewHost(
		&fakePlugin{id: "alpha", calls: &calls},
		&fakePlugin{id: "beta", startPanic: true, calls: &calls},
	)
	if err != nil {
		t.Fatalf("NewHost() error = %v, want nil", err)
	}

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
	host, err := plugin.NewHost(
		&fakePlugin{id: "alpha", stopErr: errAlpha, calls: &calls},
		&fakePlugin{id: "beta", stopPanic: true, calls: &calls},
	)
	if err != nil {
		t.Fatalf("NewHost() error = %v, want nil", err)
	}
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
