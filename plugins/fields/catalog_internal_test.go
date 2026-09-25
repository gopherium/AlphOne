// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/sdk"
)

// fakeLoader answers each tenant's catalogue with settable definitions and counts its reads.
type fakeLoader struct {
	mu    sync.Mutex
	held  map[uuid.UUID][]Definition
	err   error
	runs  map[uuid.UUID]int
	gate  chan struct{}
	enter chan struct{}
}

// liveDefinitions reports the calling tenant's settable definitions, waiting on the gate when one is set.
func (f *fakeLoader) liveDefinitions(ctx context.Context) ([]Definition, error) {
	tenant := sdk.TenantOrDefault(ctx)
	f.mu.Lock()
	f.runs[tenant]++
	gate, enter := f.gate, f.enter
	f.mu.Unlock()
	if gate != nil {
		enter <- struct{}{}
		<-gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.held[tenant], f.err
}

// readsOf reports how often the calling tenant's catalogue was read.
func (f *fakeLoader) readsOf(tenant uuid.UUID) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runs[tenant]
}

// newFakeLoader returns a loader holding no definitions for any tenant.
func newFakeLoader() *fakeLoader {
	return &fakeLoader{held: map[uuid.UUID][]Definition{}, runs: map[uuid.UUID]int{}}
}

// clock is a settable time source.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

// read reports the settable time.
func (c *clock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// pass moves the settable time forward.
func (c *clock) pass(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

// defined builds a live definition for a catalogue test.
func defined(t *testing.T, name, declared string) Definition {
	t.Helper()
	definition, err := newDefinition(name, "Label", declared, nil)
	if err != nil {
		t.Fatalf("newDefinition(%q) error = %v, want nil", name, err)
	}
	return definition
}

// inTenantOf returns a context serving a fresh tenant and that tenant.
func inTenantOf(t *testing.T) (context.Context, uuid.UUID) {
	t.Helper()
	tenant := uuid.Must(uuid.NewV7())
	return sdk.WithTenant(t.Context(), tenant), tenant
}

// mustView reads the calling tenant's view, failing the test on an error.
func mustView(t *testing.T, held *catalog, ctx context.Context) *view {
	t.Helper()
	read, err := held.viewFor(ctx)
	if err != nil {
		t.Fatalf("viewFor() error = %v, want nil", err)
	}
	return read
}

func TestCatalogServesEachTenantItsOwnDefinitionsAsGraphFields(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	inFirst, first := inTenantOf(t)
	inSecond, second := inTenantOf(t)
	loader.held[first] = []Definition{defined(t, "birthDate", "DATE")}
	loader.held[second] = []Definition{defined(t, "shoeSize", "NUMBER")}
	held := newCatalog(loader, 0, 0)

	firstView := mustView(t, held, inFirst)
	secondView := mustView(t, held, inSecond)

	wanted := sdk.GraphField{Entity: "Contact", Name: "birthDate", Type: "Date"}
	if len(firstView.fields) != 1 || firstView.fields[0] != wanted {
		t.Errorf("first tenant fields = %+v, want birthDate answering Date on Contact", firstView.fields)
	}
	if firstView.kinds["shoeSize"] != "" || firstView.kinds["birthDate"] != kindDate {
		t.Errorf("first tenant kinds = %v, want only birthDate", firstView.kinds)
	}
	if len(secondView.fields) != 1 || secondView.fields[0].Name != "shoeSize" || secondView.fields[0].Type != "Int" {
		t.Errorf("second tenant fields = %+v, want shoeSize answering Int", secondView.fields)
	}
}

func TestCatalogReadsATenantOnceUntilItIsForgotten(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	held := newCatalog(loader, 0, 0)

	mustView(t, held, ctx)
	mustView(t, held, ctx)
	if got := loader.readsOf(tenant); got != 1 {
		t.Fatalf("reads = %d, want 1 while the view is held", got)
	}
	held.forget(ctx)
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2 once the view was forgotten", got)
	}
}

func TestCatalogForgetsOnlyTheCallersView(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	inFirst, first := inTenantOf(t)
	inSecond, second := inTenantOf(t)
	held := newCatalog(loader, 0, 0)
	mustView(t, held, inFirst)
	mustView(t, held, inSecond)

	held.forget(inFirst)
	mustView(t, held, inFirst)
	mustView(t, held, inSecond)

	if got := loader.readsOf(first); got != 2 {
		t.Errorf("first tenant reads = %d, want 2", got)
	}
	if got := loader.readsOf(second); got != 1 {
		t.Errorf("second tenant reads = %d, want 1, another tenant's forget must not touch it", got)
	}
}

func TestCatalogStoresNothingFromAFailedReadAndRetriesForTheCaller(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.held[tenant] = []Definition{defined(t, "birthDate", "DATE")}
	loader.err = errors.New("catalogue unavailable")
	held := newCatalog(loader, 0, 0)

	if _, err := held.viewFor(ctx); err == nil {
		t.Fatal("viewFor() error = nil, want the failed read reported")
	}
	loader.mu.Lock()
	loader.err = nil
	loader.mu.Unlock()
	read := mustView(t, held, ctx)

	if len(read.fields) != 1 || read.fields[0].Name != "birthDate" {
		t.Errorf("fields = %+v, want the caller's own definitions read again", read.fields)
	}
	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2 for the caller's tenant", got)
	}
}

func TestCatalogNeverRepeatsAStamp(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	inFirst, _ := inTenantOf(t)
	inSecond, _ := inTenantOf(t)
	held := newCatalog(loader, 0, 0)

	before := mustView(t, held, inFirst).stamp
	other := mustView(t, held, inSecond).stamp
	held.forget(inFirst)
	after := mustView(t, held, inFirst).stamp

	if before == after || before == other || after == other {
		t.Errorf("stamps = %d, %d and %d, want every read stamped apart", before, other, after)
	}
}

func TestCatalogLetsTheTenantServedLeastRecentlyGo(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	inFirst, first := inTenantOf(t)
	inSecond, second := inTenantOf(t)
	inThird, _ := inTenantOf(t)
	held := newCatalog(loader, 2, 0)
	mustView(t, held, inFirst)
	mustView(t, held, inSecond)

	mustView(t, held, inThird)
	mustView(t, held, inSecond)
	mustView(t, held, inFirst)

	if got := loader.readsOf(first); got != 2 {
		t.Errorf("first tenant reads = %d, want 2 after it was let go", got)
	}
	if got := loader.readsOf(second); got != 1 {
		t.Errorf("second tenant reads = %d, want 1 while it stays held", got)
	}
}

func TestCatalogHoldsTheDefaultNumberOfTenantsWhenGivenNone(t *testing.T) {
	t.Parallel()

	for _, given := range []int{0, -1} {
		t.Run(fmt.Sprint(given), func(t *testing.T) {
			t.Parallel()

			loader := newFakeLoader()
			inFirst, first := inTenantOf(t)
			held := newCatalog(loader, given, 0)
			mustView(t, held, inFirst)
			for range sdk.DefaultTenantsHeld - 1 {
				ctx, _ := inTenantOf(t)
				mustView(t, held, ctx)
			}
			mustView(t, held, inFirst)
			if got := loader.readsOf(first); got != 1 {
				t.Fatalf("first tenant reads = %d, want 1 while the default number is not passed", got)
			}

			for range sdk.DefaultTenantsHeld {
				ctx, _ := inTenantOf(t)
				mustView(t, held, ctx)
			}
			mustView(t, held, inFirst)

			if got := loader.readsOf(first); got != 2 {
				t.Errorf("first tenant reads = %d, want 2 once the default number was passed", got)
			}
		})
	}
}

func TestCatalogReadsATenantAgainOnceItsViewOutlivesTheRefresh(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	now := &clock{now: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)}
	held := newCatalog(loader, 0, 10*time.Second)
	held.now = now.read
	mustView(t, held, ctx)

	now.pass(9 * time.Second)
	mustView(t, held, ctx)
	if got := loader.readsOf(tenant); got != 1 {
		t.Fatalf("reads = %d, want 1 before the refresh passes", got)
	}
	now.pass(time.Second)
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2 once the refresh passed", got)
	}
}

func TestCatalogRefreshesAfterTheDefaultPeriodWhenGivenNone(t *testing.T) {
	t.Parallel()

	for _, given := range []time.Duration{0, -time.Second} {
		t.Run(given.String(), func(t *testing.T) {
			t.Parallel()

			loader := newFakeLoader()
			ctx, tenant := inTenantOf(t)
			now := &clock{now: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)}
			held := newCatalog(loader, 0, given)
			held.now = now.read
			mustView(t, held, ctx)

			now.pass(sdk.DefaultTenantsRefresh - time.Second)
			mustView(t, held, ctx)
			if got := loader.readsOf(tenant); got != 1 {
				t.Fatalf("reads = %d, want 1 before the default refresh passes", got)
			}
			now.pass(time.Second)
			mustView(t, held, ctx)

			if got := loader.readsOf(tenant); got != 2 {
				t.Errorf("reads = %d, want 2 once the default refresh passed", got)
			}
		})
	}
}

func TestCatalogWaitsForAForgetUnderWayBeforeStoringARead(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate = make(chan struct{})
	loader.enter = make(chan struct{})
	held := newCatalog(loader, 0, 0)
	done := make(chan error)
	go func() {
		_, err := held.viewFor(ctx)
		done <- err
	}()

	<-loader.enter
	held.storing.Lock()
	close(loader.gate)
	select {
	case <-done:
		held.storing.Unlock()
		t.Fatal("viewFor() returned while a forget held the storing lock, want it to wait")
	case <-time.After(100 * time.Millisecond):
	}
	held.forgets.Add(1)
	held.views.Remove(tenant)
	held.storing.Unlock()
	if err := <-done; err != nil {
		t.Fatalf("viewFor() error = %v, want nil", err)
	}
	loader.mu.Lock()
	loader.gate = nil
	loader.mu.Unlock()
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2, a read that waited out a forget must not be kept", got)
	}
}

func TestCatalogKeepsAReadThatAForgetOvertookOutOfTheCache(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate = make(chan struct{})
	loader.enter = make(chan struct{})
	held := newCatalog(loader, 0, 0)
	done := make(chan error)
	go func() {
		_, err := held.viewFor(ctx)
		done <- err
	}()

	<-loader.enter
	held.forget(ctx)
	close(loader.gate)
	if err := <-done; err != nil {
		t.Fatalf("viewFor() error = %v, want nil", err)
	}
	loader.mu.Lock()
	loader.gate = nil
	loader.mu.Unlock()
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2, a read a forget overtook must not be kept", got)
	}
}
