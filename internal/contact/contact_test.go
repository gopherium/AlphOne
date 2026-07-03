// SPDX-License-Identifier: AGPL-3.0-or-later

package contact_test

import (
	"errors"
	"testing"

	"github.com/gopherium/alphone/internal/contact"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		name    string
		want    string
		wantErr error
	}{
		"valid name":         {name: "María Pérez", want: "María Pérez"},
		"surrounding spaces": {name: "  María Pérez  ", want: "María Pérez"},
		"empty name":         {name: "", wantErr: contact.ErrEmptyName},
		"whitespace only":    {name: " \t ", wantErr: contact.ErrEmptyName},
	}

	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got, err := contact.New(tc.name)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("New(%q) error = %v, want %v", tc.name, err, tc.wantErr)
			}
			if got.Name != tc.want {
				t.Errorf("New(%q).Name = %q, want %q", tc.name, got.Name, tc.want)
			}
		})
	}
}
