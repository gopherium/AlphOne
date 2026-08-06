// SPDX-License-Identifier: Elastic-2.0

package graphres

import (
	"context"
	"errors"
	"net/http"
)

// errNoHTTPContext reports an auth resolver running outside an HTTP request.
var errNoHTTPContext = errors.New("graphres: no http transport on context")

// httpKey keys the HTTP transport pair on the context.
type httpKey struct{}

// httpCarrier holds the transport pair of the serving request.
type httpCarrier struct {
	writer  http.ResponseWriter
	request *http.Request
}

// WithHTTP returns ctx carrying the transport pair for the auth resolvers.
func WithHTTP(ctx context.Context, w http.ResponseWriter, r *http.Request) context.Context {
	return context.WithValue(ctx, httpKey{}, httpCarrier{writer: w, request: r})
}

// httpFrom returns the transport pair installed on ctx.
func httpFrom(ctx context.Context) (httpCarrier, error) {
	carrier, ok := ctx.Value(httpKey{}).(httpCarrier)
	if !ok {
		return httpCarrier{}, errNoHTTPContext
	}
	return carrier, nil
}

// clientIPKey keys the resolved client IP on the context.
type clientIPKey struct{}

// WithClientIP returns ctx carrying the rate limiting key of the caller.
func WithClientIP(ctx context.Context, ip string) context.Context {
	return context.WithValue(ctx, clientIPKey{}, ip)
}

// clientIPFrom returns the rate limiting key installed on ctx.
func clientIPFrom(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}
