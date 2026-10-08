// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gopherium/alphone/internal/server"
)

func TestTheGraphBoundsFallBackToTheirDefaults(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL": "postgres://localhost/x",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	cfg := held.serverConfig()
	if cfg.Graph != server.DefaultGraphBounds {
		t.Errorf("graph bounds = %+v, want the defaults %+v", cfg.Graph, server.DefaultGraphBounds)
	}
	lifetime, perUser := server.DefaultMaxStreamLifetime, server.DefaultMaxStreamsPerUser
	if cfg.MaxStreamLifetime != lifetime || cfg.MaxStreamsPerUser != perUser {
		t.Errorf("stream bounds = (%v, %d), want the defaults (%v, %d)",
			cfg.MaxStreamLifetime, cfg.MaxStreamsPerUser, lifetime, perUser)
	}
}

func TestTheGraphBoundsAreReadFromTheEnvironment(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testGetenv(map[string]string{
		"ALPHONE_DATABASE_URL":              "postgres://localhost/x",
		"ALPHONE_GRAPH_OPERATIONS_PER_USER": "12",
		"ALPHONE_GRAPH_OPERATION_TIMEOUT":   "45s",
		"ALPHONE_GRAPH_BODY_MAX_BYTES":      "524288",
		"ALPHONE_GRAPH_UPLOAD_MAX_BYTES":    "3145728",
		"ALPHONE_GRAPH_RETRY_AFTER":         "2s",
		"ALPHONE_STREAMS_PER_USER":          "4",
		"ALPHONE_STREAM_LIFETIME":           "3m",
	}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	cfg := held.serverConfig()
	want := server.GraphBounds{
		OperationsPerUser: 12, OperationTimeout: 45 * time.Second,
		BodyMaxBytes: 524288, UploadMaxBytes: 3145728, RetryAfter: 2 * time.Second,
	}
	if cfg.Graph != want {
		t.Errorf("graph bounds = %+v, want %+v", cfg.Graph, want)
	}
	if cfg.MaxStreamLifetime != 3*time.Minute || cfg.MaxStreamsPerUser != 4 {
		t.Errorf("stream bounds = (%v, %d), want (3m0s, 4)", cfg.MaxStreamLifetime, cfg.MaxStreamsPerUser)
	}
}

func TestTheGraphBoundsRefuseAnUnreadableValue(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		named string
		value string
	}{
		"no operation slot":          {"ALPHONE_GRAPH_OPERATIONS_PER_USER", "0"},
		"operation slots in words":   {"ALPHONE_GRAPH_OPERATIONS_PER_USER", "twenty"},
		"no operation time":          {"ALPHONE_GRAPH_OPERATION_TIMEOUT", "0s"},
		"an operation time unitless": {"ALPHONE_GRAPH_OPERATION_TIMEOUT", "60"},
		"no body at all":             {"ALPHONE_GRAPH_BODY_MAX_BYTES", "0"},
		"a body size in units":       {"ALPHONE_GRAPH_BODY_MAX_BYTES", "1MiB"},
		"no upload at all":           {"ALPHONE_GRAPH_UPLOAD_MAX_BYTES", "0"},
		"no retry hint":              {"ALPHONE_GRAPH_RETRY_AFTER", "0s"},
		"no stream slot":             {"ALPHONE_STREAMS_PER_USER", "0"},
		"no stream time":             {"ALPHONE_STREAM_LIFETIME", "0s"},
	}
	for testName, tt := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			_, err := loadRunConfig(testGetenv(map[string]string{
				"ALPHONE_DATABASE_URL": "postgres://localhost/x",
				tt.named:               tt.value,
			}))

			if err == nil || !strings.Contains(err.Error(), tt.named) {
				t.Errorf("loadRunConfig() error = %v, want %s=%q refused by name", err, tt.named, tt.value)
			}
		})
	}
}
