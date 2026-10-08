// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"time"

	"github.com/gopherium/framework/gonsole"

	"github.com/gopherium/alphone/internal/server"
)

// graphSettings bounds the graph operations and the streams the server holds each caller to.
type graphSettings struct {
	bounds         server.GraphBounds
	streamLifetime time.Duration
	streamsPerUser int
}

// loadGraphSettings reads the graph operation bounds and the stream bounds from the environment.
func loadGraphSettings(env gonsole.Env) (graphSettings, error) {
	bounds, err := loadGraphBounds(env)
	if err != nil {
		return graphSettings{}, err
	}
	lifetime, err := env.Duration("STREAM_LIFETIME", server.DefaultMaxStreamLifetime)
	if err != nil {
		return graphSettings{}, err
	}
	perUser, err := env.Count("STREAMS_PER_USER", server.DefaultMaxStreamsPerUser)
	if err != nil {
		return graphSettings{}, err
	}
	return graphSettings{bounds: bounds, streamLifetime: lifetime, streamsPerUser: perUser}, nil
}

// loadGraphBounds reads how many graph operations a caller runs at once, how long each runs and how large its body is.
func loadGraphBounds(env gonsole.Env) (server.GraphBounds, error) {
	defaults := server.DefaultGraphBounds
	operations, err := env.Count("GRAPH_OPERATIONS_PER_USER", defaults.OperationsPerUser)
	if err != nil {
		return server.GraphBounds{}, err
	}
	timeout, err := env.Duration("GRAPH_OPERATION_TIMEOUT", defaults.OperationTimeout)
	if err != nil {
		return server.GraphBounds{}, err
	}
	body, err := env.Count("GRAPH_BODY_MAX_BYTES", int(defaults.BodyMaxBytes))
	if err != nil {
		return server.GraphBounds{}, err
	}
	upload, err := env.Count("GRAPH_UPLOAD_MAX_BYTES", int(defaults.UploadMaxBytes))
	if err != nil {
		return server.GraphBounds{}, err
	}
	retryAfter, err := env.Duration("GRAPH_RETRY_AFTER", defaults.RetryAfter)
	if err != nil {
		return server.GraphBounds{}, err
	}
	anonymous, err := loadAnonymousBounds(env)
	if err != nil {
		return server.GraphBounds{}, err
	}
	anonymous.OperationsPerUser = operations
	anonymous.OperationTimeout = timeout
	anonymous.BodyMaxBytes = int64(body)
	anonymous.UploadMaxBytes = int64(upload)
	anonymous.RetryAfter = retryAfter
	return anonymous, nil
}

// loadAnonymousBounds reads the body limit and the pools the graph holds callers with no identity to.
func loadAnonymousBounds(env gonsole.Env) (server.GraphBounds, error) {
	defaults := server.DefaultGraphBounds
	body, err := env.Count("GRAPH_ANONYMOUS_MAX_BYTES", int(defaults.AnonymousBodyMaxBytes))
	if err != nil {
		return server.GraphBounds{}, err
	}
	perIP, err := env.Count("GRAPH_ANONYMOUS_PER_IP", defaults.AnonymousPerIP)
	if err != nil {
		return server.GraphBounds{}, err
	}
	ceiling, err := env.Count("GRAPH_ANONYMOUS_CEILING", defaults.AnonymousCeiling)
	if err != nil {
		return server.GraphBounds{}, err
	}
	return server.GraphBounds{AnonymousBodyMaxBytes: int64(body), AnonymousPerIP: perIP, AnonymousCeiling: ceiling}, nil
}
