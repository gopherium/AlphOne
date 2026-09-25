// SPDX-License-Identifier: Elastic-2.0

package fields

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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
	gated chan gatedRead
}

// gatedRead is one read the loader holds open until the test releases it.
type gatedRead struct {
	ctx     context.Context
	release chan struct{}
}

// liveDefinitions reports the calling tenant's definitions as they stand when the read begins, held open while gated.
func (f *fakeLoader) liveDefinitions(ctx context.Context) ([]Definition, error) {
	tenant := sdk.TenantOrDefault(ctx)
	f.mu.Lock()
	f.runs[tenant]++
	definitions, err, gated := f.held[tenant], f.err, f.gated
	f.mu.Unlock()
	if gated == nil {
		return definitions, err
	}
	read := gatedRead{ctx: ctx, release: make(chan struct{})}
	gated <- read
	select {
	case <-read.release:
		return definitions, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// answer sets the definitions every later read of the tenant reports.
func (f *fakeLoader) answer(tenant uuid.UUID, definitions ...Definition) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.held[tenant] = definitions
}

// gate holds every later read open until the test releases it.
func (f *fakeLoader) gate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gated = make(chan gatedRead)
}

// ungate lets every later read answer at once.
func (f *fakeLoader) ungate() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gated = nil
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

// reply is what one viewFor call returned.
type reply struct {
	view *view
	err  error
}

// viewing calls viewFor in the background, handing back what it returns.
func viewing(held *catalog, ctx context.Context) <-chan reply {
	replies := make(chan reply, 1)
	go func() {
		read, err := held.viewFor(ctx)
		replies <- reply{view: read, err: err}
	}()
	return replies
}

// mustReply waits for a background viewFor call, failing the test on an error.
func mustReply(t *testing.T, replies <-chan reply) *view {
	t.Helper()
	got := <-replies
	if got.err != nil {
		t.Fatalf("viewFor() error = %v, want nil", got.err)
	}
	return got.view
}

// sharing reports how many callers wait on the tenant's read under way.
func sharing(held *catalog, tenant uuid.UUID) int {
	held.mu.Lock()
	defer held.mu.Unlock()
	if shared := held.flights[tenant]; shared != nil {
		return shared.waiting
	}
	return 0
}

// noReadStarts fails the test when another read reaches the gated loader within a moment.
func noReadStarts(t *testing.T, loader *fakeLoader) {
	t.Helper()
	select {
	case <-loader.gated:
		t.Fatal("a second read started while one was under way, want every caller to share it")
	case <-time.After(100 * time.Millisecond):
	}
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

func TestCatalogStampsEachTenantAndEachForgottenViewApart(t *testing.T) {
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
	loader.gate()
	held := newCatalog(loader, 0, 0)
	reading := viewing(held, ctx)
	read := <-loader.gated

	held.mu.Lock()
	close(read.release)
	select {
	case <-reading:
		held.mu.Unlock()
		t.Fatal("viewFor() returned while a forget held the catalogue, want it to wait")
	case <-time.After(100 * time.Millisecond):
	}
	held.drop(tenant)
	held.mu.Unlock()
	close((<-loader.gated).release)
	mustReply(t, reading)
	loader.ungate()
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2, a read that waited out a forget must not be kept", got)
	}
}

func TestCatalogKeepsAReadThatAForgetOvertookOutOfTheCache(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate()
	held := newCatalog(loader, 0, 0)
	overtaken := viewing(held, ctx)
	stale := <-loader.gated
	loader.answer(tenant, defined(t, "birthDate", "DATE"))
	held.forget(ctx)

	fresh := viewing(held, ctx)
	close((<-loader.gated).release)
	if got := mustReply(t, fresh); len(got.fields) != 1 {
		t.Fatalf("fields = %+v, want birthDate from a read started after the forget", got.fields)
	}
	close(stale.release)
	mustReply(t, overtaken)
	loader.ungate()

	if got := mustView(t, held, ctx); len(got.fields) != 1 {
		t.Errorf("fields = %+v, want the read after the forget kept over the one it overtook", got.fields)
	}
	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2, a read a forget overtook must not be kept", got)
	}
}

func TestCatalogReadsAgainForACallerWhoseReadAForgetOvertook(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate()
	held := newCatalog(loader, 0, 0)
	overtaken := viewing(held, ctx)
	stale := <-loader.gated
	loader.answer(tenant, defined(t, "birthDate", "DATE"))

	held.forget(ctx)
	if stale.ctx.Err() == nil {
		close(stale.release)
		t.Fatal("the read a forget overtook still runs, want it stopped")
	}
	close((<-loader.gated).release)

	if got := mustReply(t, overtaken); len(got.fields) != 1 || got.fields[0].Name != "birthDate" {
		t.Errorf("fields = %+v, want birthDate from the read after the forget", got.fields)
	}
}

func TestCatalogSharesOneReadAmongCallersMissingATenantTogether(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate()
	held := newCatalog(loader, 0, 0)
	first := viewing(held, ctx)
	read := <-loader.gated
	second := viewing(held, ctx)
	third := viewing(held, ctx)

	noReadStarts(t, loader)
	close(read.release)

	shared := mustReply(t, first)
	if mustReply(t, second) != shared || mustReply(t, third) != shared {
		t.Error("callers missing the tenant together got different views, want the one read they shared")
	}
	if got := loader.readsOf(tenant); got != 1 {
		t.Errorf("reads = %d, want 1 for callers missing the tenant together", got)
	}
}

func TestCatalogSharesARefreshUnderWaySoNoOlderReadLandsLast(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.held[tenant] = []Definition{defined(t, "birthDate", "DATE")}
	now := &clock{now: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)}
	held := newCatalog(loader, 0, 10*time.Second)
	held.now = now.read
	mustView(t, held, ctx)
	now.pass(10 * time.Second)
	loader.gate()
	first := viewing(held, ctx)
	refresh := <-loader.gated
	loader.answer(tenant, defined(t, "birthDate", "DATE"), defined(t, "shoeSize", "NUMBER"))

	second := viewing(held, ctx)
	noReadStarts(t, loader)
	close(refresh.release)

	shared := mustReply(t, first)
	if mustReply(t, second) != shared {
		t.Error("callers during one refresh got different views, want the refresh they shared")
	}
	loader.ungate()
	if mustView(t, held, ctx) != shared {
		t.Error("the held view is not the refresh every caller shared, want no other read landing over it")
	}
	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want 2, the first read and the one shared refresh", got)
	}
}

func TestCatalogKeepsTheStampWhenARefreshReadsTheSameFields(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.held[tenant] = []Definition{defined(t, "birthDate", "DATE")}
	now := &clock{now: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)}
	held := newCatalog(loader, 0, 10*time.Second)
	held.now = now.read
	before := mustView(t, held, ctx)

	now.pass(10 * time.Second)
	after := mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 2 {
		t.Fatalf("reads = %d, want 2 once the refresh passed", got)
	}
	if after.stamp != before.stamp {
		t.Errorf("stamp = %d after a refresh reading the same fields, want %d kept", after.stamp, before.stamp)
	}
}

func TestCatalogStampsARefreshReadingOtherFieldsAnew(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.held[tenant] = []Definition{defined(t, "birthDate", "DATE")}
	now := &clock{now: time.Date(2026, 9, 25, 9, 0, 0, 0, time.UTC)}
	held := newCatalog(loader, 0, 10*time.Second)
	held.now = now.read
	before := mustView(t, held, ctx)

	loader.answer(tenant, defined(t, "birthDate", "DATE"), defined(t, "shoeSize", "NUMBER"))
	now.pass(10 * time.Second)
	after := mustView(t, held, ctx)

	if after.stamp == before.stamp {
		t.Errorf("stamp = %d after a refresh reading other fields, want a new one", after.stamp)
	}
	if len(after.fields) != 2 {
		t.Errorf("fields = %+v, want birthDate and shoeSize", after.fields)
	}
}

func TestCatalogKeepsAReadThatAnotherTenantsForgetCrossed(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	inOther, _ := inTenantOf(t)
	loader.gate()
	held := newCatalog(loader, 0, 0)
	reading := viewing(held, ctx)
	read := <-loader.gated

	held.forget(inOther)
	close(read.release)
	mustReply(t, reading)
	loader.ungate()
	mustView(t, held, ctx)

	if got := loader.readsOf(tenant); got != 1 {
		t.Errorf("reads = %d, want 1, another tenant's forget must not cost this tenant its read", got)
	}
}

func TestCatalogFinishesASharedReadForTheCallersStillWaiting(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.held[tenant] = []Definition{defined(t, "birthDate", "DATE")}
	loader.gate()
	held := newCatalog(loader, 0, 0)
	leaving, leave := context.WithCancel(ctx)
	left := viewing(held, leaving)
	read := <-loader.gated
	staying := viewing(held, ctx)
	for sharing(held, tenant) < 2 {
		runtime.Gosched()
	}

	leave()
	if got := <-left; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("the caller that left got %v, want its own cancellation", got.err)
	}
	if err := read.ctx.Err(); err != nil {
		t.Fatalf("the shared read ended with %v, want it kept for the caller still waiting", err)
	}
	close(read.release)

	if got := mustReply(t, staying); len(got.fields) != 1 || got.fields[0].Name != "birthDate" {
		t.Errorf("fields = %+v, want the shared read's birthDate", got.fields)
	}
}

func TestCatalogStopsAReadNoCallerWaitsForAnyMore(t *testing.T) {
	t.Parallel()

	loader := newFakeLoader()
	ctx, tenant := inTenantOf(t)
	loader.gate()
	held := newCatalog(loader, 0, 0)
	leaving, leave := context.WithCancel(ctx)
	left := viewing(held, leaving)
	abandoned := <-loader.gated

	leave()
	if got := <-left; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("the caller that left got %v, want its own cancellation", got.err)
	}
	if abandoned.ctx.Err() == nil {
		t.Error("the read nobody waits for still runs, want it stopped")
	}
	next := viewing(held, ctx)
	close((<-loader.gated).release)
	mustReply(t, next)

	if got := loader.readsOf(tenant); got != 2 {
		t.Errorf("reads = %d, want a fresh read for the caller after the one that left", got)
	}
}
