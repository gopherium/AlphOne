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

	sc.When(`^the admin's session invites a user named "([^"]*)"$`, func(ctx context.Context, name string) error {
		return worldFrom(ctx).postGraphAsSession(ctx, fmt.Sprintf(
			`{"query":"mutation { invite(email: \"grace@example.com\", name: \"%s\")`+
				` { delivered } }"}`, name))
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
	registerUndeclaredRoleSteps(sc)
	registerTokenOperations(sc)
	registerErrorSteps(sc)
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
			return w.answeredWithoutError()
		})

	sc.When(`^the admin's session demotes itself to "([^"]*)"$`, func(ctx context.Context, tier string) error {
		w := worldFrom(ctx)
		return w.postGraphAsSession(ctx, settingUserRole(w.ownerID, tier))
	})

	sc.Then(`^the operation is refused as a change to its own role$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		if err := w.refusedAsOwnRole(); err != nil {
			return err
		}
		return w.standsAsAdmin(ctx, w.ownerID)
	})
}

// registerUndeclaredRoleSteps binds the steps enabling an account whose role nobody declares.
func registerUndeclaredRoleSteps(sc *godog.ScenarioContext) {
	sc.Given(`^a disabled user holding the role "([^"]*)"$`, func(ctx context.Context, held string) error {
		w := worldFrom(ctx)
		id, err := w.addUser(ctx, "held.role@example.com", "Held Role Account")
		if err != nil {
			return err
		}
		if err := w.users.SetUserRole(ctx, id, held, nil); err != nil {
			return fmt.Errorf("storing the role %q: %w", held, err)
		}
		if err := w.users.SetUserDisabled(ctx, id, true); err != nil {
			return fmt.Errorf("disabling the account: %w", err)
		}
		w.targetID = id
		return nil
	})

	sc.When(`^the admin's session enables that user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		return w.postGraphAsSession(ctx, settingUserDisabled(w.targetID, false))
	})

	sc.When(`^the admin's session gives that user the role "([^"]*)"$`,
		func(ctx context.Context, tier string) error {
			w := worldFrom(ctx)
			if err := w.postGraphAsSession(ctx, settingUserRole(w.targetID, tier)); err != nil {
				return err
			}
			return w.answeredWithoutError()
		})

	sc.Then(`^the operation is refused as beyond the admin's reach$`, func(ctx context.Context) error {
		return worldFrom(ctx).refusedAsBeyondReach()
	})

	sc.Then(`^that user is (still disabled|enabled)$`, func(ctx context.Context, state string) error {
		w := worldFrom(ctx)
		held, err := w.users.UserByID(ctx, w.targetID)
		if err != nil {
			return fmt.Errorf("reading the account: %w", err)
		}
		if want := state == "still disabled"; held.Disabled != want {
			return fmt.Errorf("disabled = %t, want %t", held.Disabled, want)
		}
		return nil
	})
}

// settingUserRole returns the operation standing one user in a tier.
func settingUserRole(userID uuid.UUID, tier string) string {
	return fmt.Sprintf(`{"query":"mutation { setUserRole(id: \"%s\", role: \"%s\") }"}`, userID, tier)
}

// settingUserDisabled returns the operation barring or admitting one user.
func settingUserDisabled(userID uuid.UUID, disabled bool) string {
	return fmt.Sprintf(`{"query":"mutation { setUserDisabled(id: \"%s\", disabled: %t) }"}`, userID, disabled)
}

// refusedAsBeyondReach reports whether the last operation was refused as an account beyond the caller's reach.
func (w *world) refusedAsBeyondReach() error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 1 {
		return fmt.Errorf("errors = %v, want exactly one error", parsed.Errors)
	}
	refused := parsed.Errors[0].Extensions
	if refused["code"] != "VALIDATION" || refused["reason"] != "role_beyond_reach" {
		return fmt.Errorf("extensions = %v, want code VALIDATION and reason role_beyond_reach", refused)
	}
	return nil
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

// refusedAsOwnRole reports whether the last operation was refused as a change to the caller's own role.
func (w *world) refusedAsOwnRole() error {
	parsed, err := w.scopeErrors()
	if err != nil {
		return err
	}
	if len(parsed.Errors) != 1 {
		return fmt.Errorf("errors = %v, want exactly one error", parsed.Errors)
	}
	refused := parsed.Errors[0]
	if code := refused.Extensions["code"]; code != "VALIDATION" {
		return fmt.Errorf("code = %v, want VALIDATION", code)
	}
	if !strings.Contains(refused.Message, "your own role") {
		return fmt.Errorf("message = %q, want it to name the caller's own role", refused.Message)
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
		return worldFrom(ctx).answeredWithoutError()
	})

	sc.Then(`^the task is answered$`, func(ctx context.Context) error {
		return worldFrom(ctx).answeredWithoutError()
	})

	sc.When(`^the member's session disables another user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		colleague, err := w.anotherUser(ctx)
		if err != nil {
			return err
		}
		return w.postGraphAsMember(ctx, settingUserDisabled(colleague, true))
	})

	sc.Then(`^that user's session may disable another user$`, func(ctx context.Context) error {
		w := worldFrom(ctx)
		colleague, err := w.anotherUser(ctx)
		if err != nil {
			return err
		}
		if err := w.postGraphAsSession(ctx, settingUserDisabled(colleague, true)); err != nil {
			return err
		}
		return w.answeredWithoutError()
	})
}

// anotherUser seeds and returns a user neither the admin nor the member is.
func (w *world) anotherUser(ctx context.Context) (uuid.UUID, error) {
	return w.addUser(ctx, "colleague@example.com", "Ada Lovelace")
}

// standsAsAdmin reports whether the named user holds the admin tier.
func (w *world) standsAsAdmin(ctx context.Context, userID uuid.UUID) error {
	held, err := w.users.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if held.Role != role.Admin.String() {
		return fmt.Errorf("the user stands in %q, want %q", held.Role, role.Admin.String())
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
