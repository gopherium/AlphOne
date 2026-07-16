// SPDX-License-Identifier: Elastic-2.0

package server

import (
	"testing"
	"time"
)

func TestStreamDefaults(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		cfg          Config
		wantLifetime time.Duration
		wantLimit    int
	}{
		"zero config falls back closed": {
			cfg:          Config{},
			wantLifetime: defaultMaxStreamLifetime,
			wantLimit:    defaultMaxStreamsPerUser,
		},
		"explicit values pass through": {
			cfg:          Config{MaxStreamLifetime: time.Minute, MaxStreamsPerUser: 2},
			wantLifetime: time.Minute,
			wantLimit:    2,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			lifetime, limit := streamDefaults(tc.cfg)

			if lifetime != tc.wantLifetime || limit != tc.wantLimit {
				t.Errorf("streamDefaults() = (%v, %d), want (%v, %d)",
					lifetime, limit, tc.wantLifetime, tc.wantLimit)
			}
		})
	}
}
