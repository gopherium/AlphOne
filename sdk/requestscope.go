// SPDX-License-Identifier: Elastic-2.0

package sdk

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// ErrNoRequestScope reports a scoped lookup on a context without a scope.
var ErrNoRequestScope = errors.New("sdk: no request scope on context")

// RequestScope carries per request values shared across resolver calls.
type RequestScope struct {
	mu     sync.Mutex
	values map[any]any
}

// NewRequestScope returns an empty request scope.
func NewRequestScope() *RequestScope {
	return &RequestScope{values: map[any]any{}}
}

// scopeKey is the context key carrying the request scope.
type scopeKey struct{}

// WithRequestScope returns ctx carrying scope.
func WithRequestScope(ctx context.Context, scope *RequestScope) context.Context {
	return context.WithValue(ctx, scopeKey{}, scope)
}

// ScopedValue returns the value under key, building it once per scope.
func ScopedValue[T any](ctx context.Context, key any, build func() T) (T, error) {
	var zero T
	scope, ok := ctx.Value(scopeKey{}).(*RequestScope)
	if !ok {
		return zero, ErrNoRequestScope
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if existing, found := scope.values[key]; found {
		typed, matches := existing.(T)
		if !matches {
			return zero, fmt.Errorf("sdk: request scope key %T holds a %T, not the requested type", key, existing)
		}
		return typed, nil
	}
	value := build()
	scope.values[key] = value
	return value, nil
}
