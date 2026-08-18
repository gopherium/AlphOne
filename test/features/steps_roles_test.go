// SPDX-License-Identifier: Elastic-2.0

package features_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
)

// identityAnswer is the answer the calling identity carries.
type identityAnswer struct {
	Data struct {
		Me struct {
			Role string `json:"role"`
		} `json:"me"`
	} `json:"data"`
}

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
	registerRoleTokenSteps(sc)
	registerRoleWriteSteps(sc)
	registerTokenOperations(sc)
	registerRefusalSteps(sc)
}

// registerRoleWriteSteps binds the steps standing a user in another tier.
func registerRoleWriteSteps(sc *godog.ScenarioContext) {
	sc.Then(`^the member's session sees its role as "([^"]*)"$`, func(ctx context.Context, tier string) error {
		w := worldFrom(ctx)
		if err := w.postGraphAsMember(ctx, `{"query":"{ me { role } }"}`); err != nil {
			return err
		}
		return w.roleSeen(tier)
	})

	sc.When(`^the admin's session promotes the member to "([^"]*)"$`,
		func(ctx context.Context, tier string) error {
			w := worldFrom(ctx)
			if err := w.postGraphAsSession(ctx, settingUserRole(w.memberID, tier)); err != nil {
				return err
			}
			return w.answeredWithoutRefusal()
		})

	sc.When(`^the admin's session demotes itself to "([^"]*)"$`, func(ctx context.Context, tier string) error {
		w := worldFrom(ctx)
		return w.postGraphAsSession(ctx, settingUserRole(w.ownerID, tier))
	})

	sc.Then(`^the operation is refused as the last admin$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.refusedAsLastAdmin(); err != nil {
			return err
		}
		return w.standsAsAdmin(ctx, w.ownerID)
	})
}

// settingUserRole returns the operation standing one user in a tier.
func settingUserRole(userID uuid.UUID, tier string) string {
	return fmt.Sprintf(`{"query":"mutation { setUserRole(id: \"%s\", role: \"%s\") }"}`, userID, tier)
}

// roleSeen reports whether the last answer carries the caller standing in one tier.
func (w *world) roleSeen(tier string) error {
	var seen identityAnswer
	if err := json.Unmarshal(w.answered, &seen); err != nil {
		return fmt.Errorf("reading the identity: %w", err)
	}
	if seen.Data.Me.Role != tier {
		return fmt.Errorf("role = %q, want %q: %s", seen.Data.Me.Role, tier, w.answered)
	}
	return nil
}

// refusedAsLastAdmin reports whether the last operation was refused for unseating the last admin.
func (w *world) refusedAsLastAdmin() error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 1 {
		return fmt.Errorf("errors = %v, want exactly one refusal", parsed.Errors)
	}
	refused := parsed.Errors[0]
	if code := refused.Extensions["code"]; code != "VALIDATION" {
		return fmt.Errorf("code = %v, want VALIDATION", code)
	}
	if !strings.Contains(refused.Message, "last admin") {
		return fmt.Errorf("message = %q, want it to name the last admin", refused.Message)
	}
	return nil
}

// registerRoleTokenSteps binds the steps holding a token to the tier of the user who minted it.
func registerRoleTokenSteps(sc *godog.ScenarioContext) {
	sc.Given(`^the (admin|member) holds a token scoped to "([^"]*)"$`,
		func(ctx context.Context, tier, scopes string) error {
			w := worldFrom(ctx)
			holder := w.ownerID
			if tier == role.Member.String() {
				holder = w.memberID
			}
			secret, err := w.mintScopedSecretFor(ctx, holder, tier, apitoken.ParseScopes(scopes))
			if err != nil {
				return err
			}
			w.scopedSecret = secret
			return nil
		})

	sc.Then(`^the operation is refused as admin only naming "([^"]*)"$`,
		func(ctx context.Context, scope string) error {
			return worldFrom(ctx).refusedAsAdminOnly(scope)
		})
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

	sc.When(`^the member's session disables another user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		colleague, err := w.anotherUser(ctx)
		if err != nil {
			return err
		}
		return w.postGraphAsMember(ctx, disablingUser(colleague))
	})

	sc.Then(`^that user's session may disable another user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		colleague, err := w.anotherUser(ctx)
		if err != nil {
			return err
		}
		if err := w.postGraphAsSession(ctx, disablingUser(colleague)); err != nil {
			return err
		}
		return w.answeredWithoutRefusal()
	})
}

// anotherUser seeds and returns a user neither the admin nor the member is.
func (w *world) anotherUser(ctx context.Context) (uuid.UUID, error) {
	return w.addUser(ctx, "colleague@example.com", "Ada Lovelace")
}

// disablingUser returns the operation disabling one user.
func disablingUser(userID uuid.UUID) string {
	return fmt.Sprintf(`{"query":"mutation { setUserDisabled(id: \"%s\", disabled: true) }"}`, userID)
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
