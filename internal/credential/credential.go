// SPDX-License-Identifier: Elastic-2.0

// Package credential carries the attribution of a request credential.
package credential

import "context"

// tokenPrefix namespaces attribution stamped from an API token.
const tokenPrefix = "token:"

// originKey is the context key carrying the credential attribution.
type originKey struct{}

// WithTokenOrigin returns ctx carrying the attribution of the named token.
func WithTokenOrigin(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, originKey{}, tokenPrefix+name)
}

// Origin returns the attribution of the request credential, or empty.
func Origin(ctx context.Context) string {
	origin, _ := ctx.Value(originKey{}).(string)
	return origin
}
