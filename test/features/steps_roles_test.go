// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
)

// userListing is the answer the session's user listing carries.
type userListing struct {
	Data struct {
		Users []struct {
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"users"`
	} `json:"data"`
}

// registerRoleSteps binds the role steps and the world lifecycle.
func registerRoleSteps(sc *godog.ScenarioContext, t *testing.T) {
	sc.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, worldKey{}, newWorld(t)), nil
	})

	sc.Given(`^a running AlphOne holding an admin and a member$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.standsAsAdmin(ctx, w.ownerID); err != nil {
			return err
		}
		member, err := w.addUser(ctx, memberEmail, "Maria Perez")
		if err != nil {
			return err
		}
		w.memberID = member
		return nil
	})

	sc.Given(`^a user provisioned before roles existed$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.standsAsAdmin(ctx, w.ownerID)
	})

	sc.When(`^the admin's session creates a user named "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).postGraphAsSession(ctx, fmt.Sprintf(
			`{"query":"mutation { createUser(email: \"grace@example.com\", name: \"%s\",`+
				` password: \"correct horse battery\") { id } }"}`, name))
	})

	sc.Then(`^the user list shows "([^"]*)" as a "([^"]*)"$`,
		func(ctx context.Context, name, tier string) error {
			w := worldFrom(ctx)
			if err := w.postGraphAsSession(ctx, `{"query":"{ users { name role } }"}`); err != nil {
				return err
			}
			return w.listingStands(name, tier)
		})

	registerMemberSteps(sc)
}

// registerMemberSteps binds the steps a member drives.
func registerMemberSteps(sc *godog.ScenarioContext) {
	sc.When(`^the member's session creates a contact named "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).postGraphAsMember(ctx, fmt.Sprintf(
			`{"query":"mutation { createContact(name: \"%s\") { id } }"}`, name))
	})

	sc.When(`^the member's session creates a task titled "([^"]*)"$`, func(ctx context.Context, title string) error {
		return worldFrom(ctx).postGraphAsMember(ctx, fmt.Sprintf(
			`{"query":"mutation { createTask(input: {title: \"%s\", dueOn: \"2026-08-20\"})`+
				` { task { id } } }"}`, title))
	})

	sc.Then(`^the contact is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutRefusal()
	})

	sc.Then(`^the task is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutRefusal()
	})

	sc.Then(`^that user's session may disable another user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		colleague, err := w.addUser(ctx, "colleague@example.com", "Ada Lovelace")
		if err != nil {
			return err
		}
		if err := w.postGraphAsSession(ctx, fmt.Sprintf(
			`{"query":"mutation { setUserDisabled(id: \"%s\", disabled: true) }"}`, colleague)); err != nil {
			return err
		}
		return w.answeredWithoutRefusal()
	})
}

// standsAsAdmin reports whether the named user holds the admin tier.
func (w *world) standsAsAdmin(ctx context.Context, userID uuid.UUID) error {
	tier, err := postgres.NewRoleStore(w.pool).RoleOf(ctx, userID)
	if err != nil {
		return err
	}
	if tier != role.Admin {
		return fmt.Errorf("the user stands in %v, want %v", tier, role.Admin)
	}
	return nil
}

// listingStands reports whether the last listing shows one user in the given tier.
func (w *world) listingStands(name, tier string) error {
	var listed userListing
	if err := json.Unmarshal(w.answered, &listed); err != nil {
		return fmt.Errorf("reading the listing: %w", err)
	}
	for _, user := range listed.Data.Users {
		if user.Name != name {
			continue
		}
		if user.Role != tier {
			return fmt.Errorf("role = %q, want %q", user.Role, tier)
		}
		return nil
	}
	return fmt.Errorf("the listing holds no user named %q: %s", name, w.answered)
}
