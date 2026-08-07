// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// counterKey keys the test counter value in the request scope.
type counterKey struct{}

// otherKey keys a second independent value in the request scope.
type otherKey struct{}

func TestScopedValueBuildsOncePerScope(t *testing.T) {
	t.Parallel()

	ctx := WithRequestScope(t.Context(), NewRequestScope())
	builds := 0

	first, err := ScopedValue(ctx, counterKey{}, func() *int { builds++; value := 1; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}
	second, err := ScopedValue(ctx, counterKey{}, func() *int { builds++; value := 2; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}

	if first != second {
		t.Error("two lookups returned different instances, want the same one")
	}
	if builds != 1 {
		t.Errorf("build ran %d times, want once", builds)
	}
}

func TestScopedValuesAreFreshAcrossScopes(t *testing.T) {
	t.Parallel()

	firstCtx := WithRequestScope(t.Context(), NewRequestScope())
	secondCtx := WithRequestScope(t.Context(), NewRequestScope())

	first, err := ScopedValue(firstCtx, counterKey{}, func() *int { value := 1; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}
	second, err := ScopedValue(secondCtx, counterKey{}, func() *int { value := 1; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}

	if first == second {
		t.Error("two scopes shared one instance, want fresh values per scope")
	}
}

func TestScopedValueKeysAreIndependent(t *testing.T) {
	t.Parallel()

	ctx := WithRequestScope(t.Context(), NewRequestScope())

	counter, err := ScopedValue(ctx, counterKey{}, func() *int { value := 1; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}
	other, err := ScopedValue(ctx, otherKey{}, func() *int { value := 2; return &value })
	if err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}

	if counter == other || *other != 2 {
		t.Errorf("independent keys collided: %v %v", *counter, *other)
	}
}

func TestScopedValueIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	ctx := WithRequestScope(t.Context(), NewRequestScope())
	var builds atomic.Int32
	results := make([]*int, 16)
	var group sync.WaitGroup

	for i := range results {
		group.Add(1)
		go func() {
			defer group.Done()
			value, err := ScopedValue(ctx, counterKey{}, func() *int {
				builds.Add(1)
				fresh := 7
				return &fresh
			})
			if err != nil {
				t.Errorf("ScopedValue() error = %v, want nil", err)
				return
			}
			results[i] = value
		}()
	}
	group.Wait()

	if builds.Load() != 1 {
		t.Errorf("build ran %d times under concurrency, want once", builds.Load())
	}
	for i, value := range results {
		if value != results[0] {
			t.Errorf("goroutine %d got a different instance", i)
		}
	}
}

func TestScopedValueFailsWithoutAScope(t *testing.T) {
	t.Parallel()

	_, err := ScopedValue(t.Context(), counterKey{}, func() *int { value := 1; return &value })

	if !errors.Is(err, ErrNoRequestScope) {
		t.Errorf("ScopedValue() error = %v, want ErrNoRequestScope", err)
	}
}

func TestScopedValueRejectsAKeyTypeMismatch(t *testing.T) {
	t.Parallel()

	ctx := WithRequestScope(t.Context(), NewRequestScope())
	if _, err := ScopedValue(ctx, counterKey{}, func() *int { value := 1; return &value }); err != nil {
		t.Fatalf("ScopedValue() error = %v, want nil", err)
	}

	_, err := ScopedValue(ctx, counterKey{}, func() string { return "mismatch" })

	if err == nil {
		t.Error("ScopedValue() error = nil, want the reused key rejected")
	}
}
