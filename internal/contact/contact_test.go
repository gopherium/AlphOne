// SPDX-License-Identifier: AGPL-3.0-or-later

package contact_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
)

var errEntropy = errors.New("entropy source failed")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) {
	return 0, errEntropy
}

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

func TestNewReportsIDGenerationFailure(t *testing.T) {
	uuid.SetRand(failingReader{})
	defer uuid.SetRand(nil)

	_, err := contact.New("María Pérez")

	if !errors.Is(err, errEntropy) {
		t.Fatalf("New() error = %v, want the entropy failure in its chain", err)
	}
}

func TestNewAssignsUniqueV7IDs(t *testing.T) {
	t.Parallel()

	first, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	second, err := contact.New("John Doe")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}

	if first.ID == uuid.Nil {
		t.Error("New().ID is uuid.Nil, want a generated UUID")
	}
	if first.ID.Version() != 7 {
		t.Errorf("New().ID version = %d, want 7", first.ID.Version())
	}
	if first.ID == second.ID {
		t.Errorf("two contacts share the ID %s, want unique IDs", first.ID)
	}
}

func TestNewSetsCreatedAtToCurrentUTCTime(t *testing.T) {
	t.Parallel()

	before := time.Now().UTC()
	got, err := contact.New("María Pérez")
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	after := time.Now().UTC()

	if got.CreatedAt.Before(before) || got.CreatedAt.After(after) {
		t.Errorf("New().CreatedAt = %v, want between %v and %v", got.CreatedAt, before, after)
	}
	if got.CreatedAt.Location() != time.UTC {
		t.Errorf("New().CreatedAt location = %v, want UTC", got.CreatedAt.Location())
	}
}
