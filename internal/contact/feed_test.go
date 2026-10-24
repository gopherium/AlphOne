// SPDX-License-Identifier: Elastic-2.0

package contact_test

import (
	"errors"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
)

func TestIdentitiesAddedSinceAsksForTheCanonicalChannel(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Transaction: 7, Identity: uuid.Must(uuid.NewV7())}
	index := &recordingIndex{
		added: []contact.AddedIdentity{{
			ContactID: uuid.Must(uuid.NewV7()), Identifier: "maria@example.com", AddedAt: time.Now(),
		}},
		next: contact.Cursor{Transaction: 9, Identity: uuid.Must(uuid.NewV7())},
	}

	got, next, err := contact.NewFeed(index, 5).IdentitiesAddedSince(t.Context(), " Email ", after, 5)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want nil", err)
	}
	if index.channel != "email" {
		t.Errorf("the store was asked for the channel %q, want email", index.channel)
	}
	if index.after != after || index.limit != 5 {
		t.Errorf("the store was asked for at most %d after %+v, want at most 5 after %+v", index.limit, index.after, after)
	}
	if diff := cmp.Diff(index.added, got); diff != "" {
		t.Errorf("IdentitiesAddedSince() mismatch (-want +got):\n%s", diff)
	}
	if next != index.next {
		t.Errorf("next cursor = %+v, want %+v", next, index.next)
	}
}

func TestIdentitiesAddedSinceAsksForNoMoreThanItsBound(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{}

	_, _, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 5)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want nil", err)
	}
	if index.limit != 2 {
		t.Errorf("the store was asked for at most %d identities, want the bound of 2", index.limit)
	}
}

func TestIdentitiesAddedSinceValidatesItsInput(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		channel contact.Channel
		limit   int
		want    error
	}{
		"blank channel": {channel: " ", limit: 1, want: contact.ErrEmptyChannel},
		"zero limit":    {channel: "email", limit: 0, want: contact.ErrInvalidLimit},
		"below zero":    {channel: "email", limit: -1, want: contact.ErrInvalidLimit},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			index := &recordingIndex{}

			_, _, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), tt.channel, contact.Cursor{}, tt.limit)

			if !errors.Is(err, tt.want) {
				t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, tt.want)
			}
			if index.reads != 0 {
				t.Errorf("the store was read %d times, want none", index.reads)
			}
		})
	}
}

func TestIdentitiesAddedSinceReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{err: errIndexStore}

	_, _, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 1)

	if !errors.Is(err, errIndexStore) {
		t.Errorf("IdentitiesAddedSince() error = %v, want the store failure", err)
	}
}

func TestIdentitiesAddedSinceRefusesACursorPastTheServerOnAnEmptyPage(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Transaction: 7, Identity: uuid.Must(uuid.NewV7())}
	index := &recordingIndex{next: after, ahead: true}

	got, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", after, 1)

	if !errors.Is(err, contact.ErrCursorAhead) {
		t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, contact.ErrCursorAhead)
	}
	if got != nil || next != after {
		t.Errorf("IdentitiesAddedSince() = %+v at %+v on a refusal, want nothing at %+v", got, next, after)
	}
}

func TestIdentitiesAddedSinceAnswersAPageWithoutCheckingTheCursor(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{
		added: []contact.AddedIdentity{{ContactID: uuid.Must(uuid.NewV7()), Identifier: "maria@example.com"}},
		ahead: true,
	}

	got, _, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 1)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want the page", err)
	}
	if len(got) != 1 || index.checks != 0 {
		t.Errorf("IdentitiesAddedSince() = %+v after %d cursor checks, want the page and no check", got, index.checks)
	}
}

func TestIdentitiesAddedSinceReportsAFailedCursorCheck(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Transaction: 7}
	index := &recordingIndex{next: after, aheadErr: errIndexStore}

	_, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", after, 1)

	if !errors.Is(err, errIndexStore) {
		t.Errorf("IdentitiesAddedSince() error = %v, want the store failure", err)
	}
	if next != after {
		t.Errorf("IdentitiesAddedSince() moved the cursor to %+v on a failure, want it kept at %+v", next, after)
	}
}

func TestHorizonAnswersTheStoreHorizon(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{horizon: contact.Cursor{Transaction: 42}}

	got, err := contact.NewFeed(index, 2).Horizon(t.Context())

	if err != nil {
		t.Fatalf("Horizon() error = %v, want nil", err)
	}
	if got != index.horizon {
		t.Errorf("Horizon() = %+v, want %+v", got, index.horizon)
	}
}

func TestHorizonReportsAStoreFailure(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{err: errIndexStore}

	if _, err := contact.NewFeed(index, 2).Horizon(t.Context()); !errors.Is(err, errIndexStore) {
		t.Errorf("Horizon() error = %v, want the store failure", err)
	}
}
