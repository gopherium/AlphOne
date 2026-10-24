// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// identityReadDeadline is how long a wiring test waits for an identity the feed still holds back.
const identityReadDeadline = 30 * time.Second

// identityReadPause is how long a wiring test waits between two reads of the feed.
const identityReadPause = 20 * time.Millisecond

// readingSeams returns plugins that read the seams run hands them through read, then stop the run.
func readingSeams(read func(sdk.Deps)) func(sdk.Deps) ([]sdk.Plugin, error) {
	return func(deps sdk.Deps) ([]sdk.Plugin, error) {
		read(deps)
		return nil, errCaptured
	}
}

// runReading runs the server over env with plugins that read the seams through read, failing unless the run stopped.
func runReading(t *testing.T, env map[string]string, read func(sdk.Deps)) {
	t.Helper()
	if err := run(t.Context(), testkit.Getenv(env), io.Discard, readingSeams(read)); !errors.Is(err, errCaptured) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errCaptured)
	}
}

// standingInNewTenant stores a tenant in the database behind pool and returns a context serving it.
func standingInNewTenant(t *testing.T, pool *pgxpool.Pool) context.Context {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, "Acme"); err != nil {
		t.Fatalf("seeding the tenant: %v", err)
	}
	return sdk.WithTenant(t.Context(), id)
}

// emailOwner is a contact's name beside every email address it owns.
type emailOwner struct {
	name   string
	emails []string
}

// storedEmailOwner stores the contact owning every email address in the tenant ctx serves and returns its id.
func storedEmailOwner(t *testing.T, pool *pgxpool.Pool, ctx context.Context, owned emailOwner) uuid.UUID {
	t.Helper()
	addresses := make([]contact.Address, 0, len(owned.emails))
	for _, email := range owned.emails {
		addresses = append(addresses, contact.Address{Channel: "email", Identifier: email})
	}
	owner, _, err := contact.NewResolver(postgres.NewContactStore(pool)).CreateWithIdentities(ctx, owned.name, addresses)
	if err != nil {
		t.Fatalf("storing %s: %v", owned.name, err)
	}
	return owner.ID
}

// settledIdentities reads the email feed past the cursor until it answers want identities or the deadline passes.
func settledIdentities(
	ctx context.Context, feed sdk.IdentityFeed, after sdk.IdentityCursor, want int,
) ([]sdk.AddedIdentity, sdk.IdentityCursor, error) {
	deadline := time.Now().Add(identityReadDeadline)
	for {
		answered, next, err := feed.IdentitiesAddedSince(ctx, "email", after, want+1)
		if err != nil {
			return nil, after, err
		}
		if len(answered) >= want || time.Now().After(deadline) {
			return answered, next, nil
		}
		time.Sleep(identityReadPause)
	}
}

// feedReads are the pages a wiring test reads from the email feed.
type feedReads struct {
	first, rest, past []sdk.AddedIdentity
}

// readFeedPages reads a settled first page from the start, the page after it, then a page from the horizon.
func readFeedPages(ctx context.Context, feed sdk.IdentityFeed) (feedReads, error) {
	var reads feedReads
	first, cursor, err := settledIdentities(ctx, feed, sdk.IdentityCursor{}, 2)
	if err != nil {
		return reads, err
	}
	reads.first = first
	if reads.rest, _, err = feed.IdentitiesAddedSince(ctx, "email", cursor, 10); err != nil {
		return reads, err
	}
	horizon, err := feed.Horizon(ctx)
	if err != nil {
		return reads, err
	}
	reads.past, _, err = feed.IdentitiesAddedSince(ctx, "email", horizon, 10)
	return reads, err
}

// wantIdentitiesOf fails the test unless answered holds exactly the want identifiers, each of owner and timed.
func wantIdentitiesOf(t *testing.T, answered []sdk.AddedIdentity, owner uuid.UUID, want []string) {
	t.Helper()
	var got []string
	for _, added := range answered {
		if added.ContactID != owner || added.AddedAt.IsZero() {
			t.Errorf("the feed answered %+v, want an identity of %s carrying the time it was added", added, owner)
		}
		got = append(got, added.Identifier)
	}
	if slices.Sort(got); !slices.Equal(got, want) {
		t.Errorf("the feed of the caller's tenant answered %v, want %v", got, want)
	}
}

func TestTheContactMatchBoundFallsBackToItsDefault(t *testing.T) {
	t.Parallel()

	held, err := loadRunConfig(testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}))

	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	if held.contactMatchMax != contact.DefaultMatchMax {
		t.Errorf("contactMatchMax = %d, want the default %d", held.contactMatchMax, contact.DefaultMatchMax)
	}
}

func TestRunHandsPluginsAMatcherOverTheirOwnTenant(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	acme := standingInNewTenant(t, pool)
	mine := storedEmailOwner(t, pool, acme, emailOwner{"Maria Perez", []string{"maria@example.com"}})
	storedEmailOwner(t, pool, t.Context(), emailOwner{"Ada Lovelace", []string{"ada@example.com"}})
	var matched map[string]sdk.Contact
	var matchErr error

	runReading(t, map[string]string{"ALPHONE_DATABASE_URL": databaseURL}, func(deps sdk.Deps) {
		matched, matchErr = deps.Matcher.MatchIdentities(acme, "email", []string{"Maria@Example.com", "ada@example.com"})
	})

	if matchErr != nil {
		t.Fatalf("MatchIdentities() error = %v, want nil", matchErr)
	}
	want := map[string]sdk.Contact{"maria@example.com": {ID: mine, Name: "Maria Perez"}}
	if !maps.Equal(matched, want) {
		t.Errorf("MatchIdentities() = %+v, want only Maria Perez of the caller's own tenant", matched)
	}
}

func TestRunHandsPluginsAMatcherHeldToTheConfiguredBound(t *testing.T) {
	t.Parallel()

	var matchErr error

	runReading(t, map[string]string{
		"ALPHONE_DATABASE_URL":      unreachableDatabaseURL,
		"ALPHONE_CONTACT_MATCH_MAX": "2",
	}, func(deps sdk.Deps) {
		_, matchErr = deps.Matcher.MatchIdentities(t.Context(), "email",
			[]string{"maria@example.com", "ada@example.com", "grace@example.com"})
	})

	if !errors.Is(matchErr, sdk.ErrTooManyIdentifiers) {
		t.Errorf("MatchIdentities() of three error = %v, want %v under a bound of two", matchErr, sdk.ErrTooManyIdentifiers)
	}
}

func TestRunHandsPluginsAFeedOverTheirOwnTenantInPagesHeldToTheBound(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	pool := testPool(t, databaseURL)
	acme := standingInNewTenant(t, pool)
	want := []string{"maria.perez@example.com", "maria@example.com", "mperez@example.com"}
	mine := storedEmailOwner(t, pool, acme, emailOwner{"Maria Perez", want})
	storedEmailOwner(t, pool, t.Context(), emailOwner{"Ada Lovelace", []string{"ada@example.com"}})
	var reads feedReads
	var readErr error

	runReading(t, map[string]string{
		"ALPHONE_DATABASE_URL":      databaseURL,
		"ALPHONE_CONTACT_MATCH_MAX": "2",
	}, func(deps sdk.Deps) {
		reads, readErr = readFeedPages(acme, deps.Identities)
	})

	if readErr != nil {
		t.Fatalf("reading the feed error = %v, want nil", readErr)
	}
	if len(reads.first) != 2 {
		t.Errorf("the first page answered %d identities, want the bound of 2", len(reads.first))
	}
	wantIdentitiesOf(t, slices.Concat(reads.first, reads.rest), mine, want)
	if len(reads.past) != 0 {
		t.Errorf("a reader starting at the horizon answered %+v, want none of the identities it settled", reads.past)
	}
}
