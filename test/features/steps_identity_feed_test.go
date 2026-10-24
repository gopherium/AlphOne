// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/sdk"
)

// createContactMutation stores one contact owning no identity through the graph.
const createContactMutation = `mutation($name: String!) { createContact(name: $name) { id } }`

// addIdentityMutation attaches one identity to a contact through the graph.
const addIdentityMutation = `mutation($contactId: UUID!, $identity: ContactIdentityInput!) {
	addContactIdentity(contactId: $contactId, identity: $identity) { id }
}`

// defaultTenant names the tenant every unplaced caller stands in.
const defaultTenant = "default"

// feedPageLimit is the most identities the scenario plugin reads in one call.
const feedPageLimit = 50

// feedSettleDeadline is how long the scenario plugin waits for an identity the feed still holds back.
const feedSettleDeadline = 30 * time.Second

// feedPause is how long the scenario plugin waits between two reads of the feed.
const feedPause = 20 * time.Millisecond

// createdContact is the envelope the contact creating step reads.
type createdContact struct {
	Data struct {
		CreateContact struct {
			ID string `json:"id"`
		} `json:"createContact"`
	} `json:"data"`
}

// feedReader is the plugin a scenario runs over the identity feed, keeping one cursor per tenant.
type feedReader struct {
	feed    sdk.IdentityFeed
	cursors map[string]sdk.IdentityCursor
}

// standingIn returns ctx serving the named tenant, the default one included.
func (w *world) standingIn(ctx context.Context, tenant string) (context.Context, error) {
	if tenant == defaultTenant {
		return sdk.WithTenant(ctx, sdk.DefaultTenantID), nil
	}
	var id uuid.UUID
	if err := w.pool.QueryRow(ctx, "SELECT id FROM core.tenants WHERE name = $1", tenant).Scan(&id); err != nil {
		return nil, fmt.Errorf("reading the tenant %q: %w", tenant, err)
	}
	return sdk.WithTenant(ctx, id), nil
}

// createContactIn stores a contact through the graph as the user placed in the tenant and remembers it.
func (w *world) createContactIn(ctx context.Context, tenant, name string) error {
	secret, err := w.secretOf(tenant)
	if err != nil {
		return err
	}
	if _, err := w.operationAs(ctx, secret, createContactMutation, map[string]any{"name": name}); err != nil {
		return err
	}
	var answer createdContact
	if err := json.Unmarshal(w.answered, &answer); err != nil {
		return fmt.Errorf("decoding %s: %w", w.answered, err)
	}
	id, err := uuid.Parse(answer.Data.CreateContact.ID)
	if err != nil {
		return fmt.Errorf("the tenant %q could not store %q, answered %s", tenant, name, w.answered)
	}
	w.lastContact = id
	return nil
}

// addEmailIn attaches an email identity to the last contact through the graph as the user placed in the tenant.
func (w *world) addEmailIn(ctx context.Context, tenant, address string) error {
	secret, err := w.secretOf(tenant)
	if err != nil {
		return err
	}
	answer, err := w.operationAs(ctx, secret, addIdentityMutation, map[string]any{
		"contactId": w.lastContact.String(),
		"identity":  map[string]any{"channel": "email", "identifier": address},
	})
	if err != nil {
		return err
	}
	if len(answer.Errors) > 0 {
		return fmt.Errorf("the tenant %q could not add %q, answered %s", tenant, address, w.answered)
	}
	return nil
}

// startFeedReader starts the scenario plugin at the feed horizon of the default tenant and every placed one.
func (w *world) startFeedReader(ctx context.Context) error {
	reader := &feedReader{
		feed:    scenarioFeed{feed: contact.NewFeed(w.contacts, feedPageLimit)},
		cursors: map[string]sdk.IdentityCursor{},
	}
	tenants := []string{defaultTenant}
	for tenant := range w.members {
		tenants = append(tenants, tenant)
	}
	for _, tenant := range tenants {
		standing, err := w.standingIn(ctx, tenant)
		if err != nil {
			return err
		}
		horizon, err := reader.feed.Horizon(standing)
		if err != nil {
			return fmt.Errorf("reading the horizon of %q: %w", tenant, err)
		}
		reader.cursors[tenant] = horizon
	}
	w.reader = reader
	return nil
}

// readFeedOf returns the email identities the feed answers the tenant past its cursor.
func (w *world) readFeedOf(ctx context.Context, tenant string) ([]sdk.AddedIdentity, error) {
	if w.reader == nil {
		return nil, fmt.Errorf("the scenario started no feed reader")
	}
	standing, err := w.standingIn(ctx, tenant)
	if err != nil {
		return nil, err
	}
	answered, _, err := w.reader.feed.IdentitiesAddedSince(standing, "email", w.reader.cursors[tenant], feedPageLimit)
	if err != nil {
		return nil, fmt.Errorf("reading the feed of %q: %w", tenant, err)
	}
	return answered, nil
}

// addedTo is an identifier the feed answered beside the contact that gained it.
type addedTo struct {
	identifier string
	contact    uuid.UUID
}

// addedToEach returns the identifier and the contact of every answered identity, in order.
func addedToEach(answered []sdk.AddedIdentity) []addedTo {
	pairs := make([]addedTo, len(answered))
	for i, added := range answered {
		pairs[i] = addedTo{added.Identifier, added.ContactID}
	}
	return pairs
}

// settledFeedOf reads the feed of the tenant until it answers an identity or the deadline passes.
func (w *world) settledFeedOf(ctx context.Context, tenant string) ([]sdk.AddedIdentity, error) {
	deadline := time.Now().Add(feedSettleDeadline)
	for {
		answered, err := w.readFeedOf(ctx, tenant)
		if err != nil {
			return nil, err
		}
		if len(answered) > 0 || time.Now().After(deadline) {
			return answered, nil
		}
		time.Sleep(feedPause)
	}
}

// bindIdentityFeedSteps binds the identity feed steps onto an already booted world.
func bindIdentityFeedSteps(sc *godog.ScenarioContext) {
	sc.Given(`^the tenant "([^"]*)" holds the contact "([^"]*)"$`, func(ctx context.Context, tenant, name string) error {
		return worldFrom(ctx).createContactIn(ctx, tenant, name)
	})

	sc.Given(`^a plugin reads the email feed of each tenant from its horizon$`, func(ctx context.Context) error {
		return worldFrom(ctx).startFeedReader(ctx)
	})

	sc.When(`^the tenant "([^"]*)" adds the email address "([^"]*)" to that contact$`,
		func(ctx context.Context, tenant, address string) error {
			return worldFrom(ctx).addEmailIn(ctx, tenant, address)
		})

	sc.Then(`^the email feed of the tenant "([^"]*)" answers "([^"]*)" for that contact$`,
		func(ctx context.Context, tenant, address string) error {
			w := worldFrom(ctx)
			answered, err := w.settledFeedOf(ctx, tenant)
			if err != nil {
				return err
			}
			want := []addedTo{{address, w.lastContact}}
			if got := addedToEach(answered); !slices.Equal(got, want) {
				return fmt.Errorf("the feed of %q answered %+v, want only %q for the contact %s",
					tenant, answered, address, w.lastContact)
			}
			return nil
		})

	sc.Then(`^the email feed of the default tenant answers no address$`, func(ctx context.Context) error {
		answered, err := worldFrom(ctx).readFeedOf(ctx, defaultTenant)
		if err != nil {
			return err
		}
		if len(answered) > 0 {
			return fmt.Errorf("the feed of the default tenant answered %+v, want no address", answered)
		}
		return nil
	})
}
