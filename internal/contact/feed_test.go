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

// serverOfTheCursor identifies the database server a unit test cursor names.
const serverOfTheCursor = 184467235

// waitingPage returns one identity waiting past a cursor.
func waitingPage() []contact.AddedIdentity {
	return []contact.AddedIdentity{{ContactID: uuid.Must(uuid.NewV7()), Identifier: "maria@example.com"}}
}

func TestIdentitiesAddedSinceRefusesAStaleCursorBeforeReading(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Server: serverOfTheCursor, Transaction: 7, Identity: uuid.Must(uuid.NewV7())}
	tests := map[string]contact.CursorCheck{
		"a cursor past the server":         {Server: serverOfTheCursor, Ahead: true},
		"a cursor another server numbered": {Server: serverOfTheCursor + 1},
	}

	for name, check := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			index := &recordingIndex{added: waitingPage(), check: check}

			got, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", after, 1)

			if !errors.Is(err, contact.ErrCursorStale) {
				t.Errorf("IdentitiesAddedSince() error = %v, want %v", err, contact.ErrCursorStale)
			}
			if got != nil || next != after {
				t.Errorf("IdentitiesAddedSince() = %+v at %+v on a refusal, want nothing at %+v", got, next, after)
			}
			if index.reads != 0 {
				t.Errorf("the store was read %d times, want none past a stale cursor", index.reads)
			}
		})
	}
}

func TestIdentitiesAddedSinceTakesTheZeroCursorOnAnyServer(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{added: waitingPage(), check: contact.CursorCheck{Server: serverOfTheCursor}}

	got, _, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 1)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want the page", err)
	}
	if len(got) != 1 || index.checks != 1 {
		t.Errorf("IdentitiesAddedSince() = %+v after %d cursor checks, want the page after one check", got, index.checks)
	}
}

func TestIdentitiesAddedSinceNamesTheServerInTheCursorAfterAPage(t *testing.T) {
	t.Parallel()

	index := &recordingIndex{
		added: waitingPage(),
		next:  contact.Cursor{Transaction: 9, Identity: uuid.Must(uuid.NewV7())},
		check: contact.CursorCheck{Server: serverOfTheCursor},
	}

	_, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", contact.Cursor{}, 1)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want the page", err)
	}
	want := contact.Cursor{Server: serverOfTheCursor, Transaction: 9, Identity: index.next.Identity}
	if next != want {
		t.Errorf("next cursor = %+v, want %+v", next, want)
	}
}

func TestIdentitiesAddedSinceAnswersTheGivenCursorOnAnEmptyPage(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Server: serverOfTheCursor, Transaction: 7, Identity: uuid.Must(uuid.NewV7())}
	index := &recordingIndex{check: contact.CursorCheck{Server: serverOfTheCursor}}

	got, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", after, 1)

	if err != nil {
		t.Fatalf("IdentitiesAddedSince() error = %v, want nil", err)
	}
	if got != nil || next != after {
		t.Errorf("IdentitiesAddedSince() = %+v at %+v, want nothing at %+v", got, next, after)
	}
}

func TestIdentitiesAddedSinceReportsAFailedCursorCheckBeforeReading(t *testing.T) {
	t.Parallel()

	after := contact.Cursor{Server: serverOfTheCursor, Transaction: 7}
	index := &recordingIndex{added: waitingPage(), checkErr: errIndexStore}

	got, next, err := contact.NewFeed(index, 2).IdentitiesAddedSince(t.Context(), "email", after, 1)

	if !errors.Is(err, errIndexStore) {
		t.Errorf("IdentitiesAddedSince() error = %v, want the store failure", err)
	}
	if got != nil || next != after {
		t.Errorf("IdentitiesAddedSince() = %+v at %+v on a failure, want nothing at %+v", got, next, after)
	}
	if index.reads != 0 {
		t.Errorf("the store was read %d times, want none after a failed check", index.reads)
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
