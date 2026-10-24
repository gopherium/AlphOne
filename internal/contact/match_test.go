// SPDX-License-Identifier: Elastic-2.0

package contact_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
)

var _ contact.MatchStore = (*recordingIndex)(nil)

var _ contact.FeedStore = (*recordingIndex)(nil)

var errIndexStore = errors.New("the identity index is unreachable")

// recordingIndex answers fixed results and records what each read asked for.
type recordingIndex struct {
	matches     map[string]contact.Contact
	added       []contact.AddedIdentity
	next        contact.Cursor
	horizon     contact.Cursor
	err         error
	check       contact.CursorCheck
	checkErr    error
	reads       int
	checks      int
	channel     contact.Channel
	identifiers []string
	after       contact.Cursor
	limit       int
}

// MatchIdentities records the channel and identifiers and answers the fixed matches.
func (r *recordingIndex) MatchIdentities(
	_ context.Context, channel contact.Channel, identifiers []string,
) (map[string]contact.Contact, error) {
	r.reads++
	r.channel, r.identifiers = channel, identifiers
	return r.matches, r.err
}

// IdentitiesAddedSince records the channel, cursor and limit and answers the fixed page.
func (r *recordingIndex) IdentitiesAddedSince(
	_ context.Context, channel contact.Channel, after contact.Cursor, limit int,
) ([]contact.AddedIdentity, contact.Cursor, error) {
	r.reads++
	r.channel, r.after, r.limit = channel, after, limit
	return r.added, r.next, r.err
}

// IdentityHorizon answers the fixed horizon.
func (r *recordingIndex) IdentityHorizon(context.Context) (contact.Cursor, error) {
	r.reads++
	return r.horizon, r.err
}

// IdentityCursorCheck counts the check and answers the fixed check.
func (r *recordingIndex) IdentityCursorCheck(context.Context, contact.Cursor) (contact.CursorCheck, error) {
	r.checks++
	return r.check, r.checkErr
}

func TestAMatchAnswersAHundredIdentifiersByDefault(t *testing.T) {
	t.Parallel()

	if contact.DefaultMatchMax != 100 {
		t.Errorf("DefaultMatchMax = %d, want 100", contact.DefaultMatchMax)
	}
}

func TestMatchIdentitiesAsksForTheCanonicalForms(t *testing.T) {
	t.Parallel()

	maria := contact.Contact{ID: uuid.Must(uuid.NewV7()), Name: "Maria Perez"}
	index := &recordingIndex{matches: map[string]contact.Contact{"maria@example.com": maria}}

	got, err := contact.NewMatcher(index, 2).MatchIdentities(
		t.Context(), " Email ", []string{"Maria@Example.COM", " ADA@example.com "},
	)

	if err != nil {
		t.Fatalf("MatchIdentities() error = %v, want nil", err)
	}
	if index.channel != "email" {
		t.Errorf("the store was asked for channel %q, want email", index.channel)
	}
	if diff := cmp.Diff([]string{"maria@example.com", "ada@example.com"}, index.identifiers); diff != "" {
		t.Errorf("the store was asked for identifiers (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(index.matches, got); diff != "" {
		t.Errorf("MatchIdentities() mismatch (-want +got):\n%s", diff)
	}
}

func TestMatchIdentitiesRefusesABatchAboveItsBoundWithoutReading(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{}

	_, err := contact.NewMatcher(index, 2).MatchIdentities(
		t.Context(), "email", []string{"maria@example.com", "ada@example.com", "grace@example.com"},
	)

	if !errors.Is(err, contact.ErrTooManyIdentifiers) {
		t.Errorf("MatchIdentities() error = %v, want %v", err, contact.ErrTooManyIdentifiers)
	}
	if index.reads != 0 {
		t.Errorf("the store was read %d times, want none", index.reads)
	}
}

func TestMatchIdentitiesValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		channel     contact.Channel
		identifiers []string
		want        error
	}{
		"blank channel": {
			channel: " ", identifiers: []string{"maria@example.com"}, want: contact.ErrEmptyChannel,
		},
		"blank identifier": {
			channel: "email", identifiers: []string{"maria@example.com", " "}, want: contact.ErrEmptyIdentifier,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			index := &recordingIndex{}

			_, err := contact.NewMatcher(index, 2).MatchIdentities(t.Context(), tt.channel, tt.identifiers)

			if !errors.Is(err, tt.want) {
				t.Errorf("MatchIdentities() error = %v, want %v", err, tt.want)
			}
			if index.reads != 0 {
				t.Errorf("the store was read %d times, want none", index.reads)
			}
		})
	}
}

func TestMatchIdentitiesReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{err: errIndexStore}

	_, err := contact.NewMatcher(index, 2).MatchIdentities(t.Context(), "email", []string{"maria@example.com"})

	if !errors.Is(err, errIndexStore) {
		t.Errorf("MatchIdentities() error = %v, want the store failure", err)
	}
}
