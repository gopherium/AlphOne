// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/framework/goncierge"
	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
)

// stewardRole is the plugin declared role standing above admin in these tests.
const stewardRole role.Role = "steward"

// retiredRole is a role an account still holds after every source stopped declaring it.
const retiredRole = "auditor"

// errListing reports a store that cannot list the accounts.
var errListing = errors.New("accounts unavailable")

// errWriting reports a store that cannot store a role.
var errWriting = errors.New("role unwritable")

// stewardRoles returns the core registry beside the steward role a plugin grants above admin.
func stewardRoles() *goncierge.Registry {
	roles := role.NewRegistry()
	if err := roles.Grant("steward", stewardRole.String(),
		role.ManageUsers, role.ManageWebhooks, "manage_reports"); err != nil {
		panic(err)
	}
	return roles
}

// adminOver returns the account administration over store, guarded and checked against roles as production builds it.
func adminOver(store authkit.AdminStore, roles *goncierge.Registry) *authkit.AdminHandlers {
	return authkit.NewAdmin(authkit.AdminConfig{
		Store: store, Privileged: roles.HoldersOf(role.ManageUsers), Roles: roles,
	})
}

// roledStore returns a store holding one account under the given tier.
func roledStore(t *testing.T, tier role.Role) (*testkit.Store, gouncer.User) {
	t.Helper()
	store := testkit.NewStore()
	held := store.AddUser(t, "grace@example.com", "Grace Hopper", testPassword)
	held.Role = tier.String()
	store.Users[held.ID] = held
	return store, held
}

// newRoledResolver returns a resolver over a store holding one account under the tier.
func newRoledResolver(t *testing.T, tier role.Role) (*graphres.Resolver, gouncer.User) {
	t.Helper()
	store, held := roledStore(t, tier)
	return newAuthResolver(store), held
}

// newActingClient returns a graph client acting as the given identity.
func newActingClient(t *testing.T, resolver *graphres.Resolver, actor authkit.Identity) *gqlclient.Client {
	t.Helper()
	return newDecoratedGraphClient(t, resolver, func(ctx context.Context) context.Context {
		return authkit.WithIdentity(ctx, actor)
	})
}

// settingRole returns the mutation standing a user in a tier.
func settingRole(userID uuid.UUID, tier string) string {
	return fmt.Sprintf(`mutation { setUserRole(id: %q, role: %q) }`, userID, tier)
}

// settingDisabled returns the mutation barring or admitting a user.
func settingDisabled(userID uuid.UUID, disabled bool) string {
	return fmt.Sprintf(`mutation { setUserDisabled(id: %q, disabled: %t) }`, userID, disabled)
}

// refusal is the first error a refused operation answers.
type refusal struct {
	Message    string         `json:"message"`
	Extensions map[string]any `json:"extensions"`
}

// refusalOf returns the first error a raw answer carries, failing the test when it carries none.
func refusalOf(t *testing.T, raw json.RawMessage) refusal {
	t.Helper()
	var refused []refusal
	if err := json.Unmarshal(raw, &refused); err != nil || len(refused) == 0 {
		t.Fatalf("errors = %s, want the operation refused", raw)
	}
	return refused[0]
}

// wantUnknownRole checks the first error answers an undeclared role with authkit's message, reason and field.
func wantUnknownRole(t *testing.T, raw json.RawMessage) {
	t.Helper()
	refused := refusalOf(t, raw)
	if got := refused.Message; got != "unknown role" {
		t.Errorf("message = %q, want unknown role", got)
	}
	if got := refused.Extensions["reason"]; got != "role_unknown" {
		t.Errorf("reason = %v, want role_unknown", got)
	}
	meta, _ := refused.Extensions["meta"].(map[string]any)
	if meta["field"] != "role" {
		t.Errorf("meta = %v, want the field the undeclared role came in named as data", meta)
	}
}

// wantBeyondReach checks the first error answers a role beyond the caller's reach in the deployment's own voice.
func wantBeyondReach(t *testing.T, raw json.RawMessage) {
	t.Helper()
	refused := refusalOf(t, raw)
	if got := refused.Message; got != "that role is beyond your own" {
		t.Errorf("message = %q, want that role is beyond your own", got)
	}
	if got := refused.Extensions["code"]; got != "VALIDATION" {
		t.Errorf("code = %v, want VALIDATION", got)
	}
	if got := refused.Extensions["reason"]; got != "role_beyond_reach" {
		t.Errorf("reason = %v, want role_beyond_reach", got)
	}
}

func TestUsersCarriesEachAccountsTier(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, role.Admin)
	client := newGraphClient(t, resolver, held.ID)

	var listed struct {
		Users []struct {
			Role string `json:"role"`
		} `json:"users"`
	}
	client.MustPost(`{ users { role } }`, &listed)

	if len(listed.Users) == 0 {
		t.Fatal("users answered nothing, want the seeded account")
	}
	if listed.Users[0].Role != role.Admin.String() {
		t.Errorf("role = %q, want %q", listed.Users[0].Role, role.Admin.String())
	}
}

func TestUsersReportsAnAccountHoldingNoRoleAsHoldingNone(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, "")
	client := newGraphClient(t, resolver, held.ID)

	var listed struct {
		Users []struct {
			Role string `json:"role"`
		} `json:"users"`
	}
	client.MustPost(`{ users { role } }`, &listed)

	if len(listed.Users) == 0 {
		t.Fatal("users answered nothing, want the seeded account")
	}
	if listed.Users[0].Role != "" {
		t.Errorf("role = %q, want it empty rather than a role the account does not hold",
			listed.Users[0].Role)
	}
}

func TestMeAnswersTheCallersTier(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, role.Admin)
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	var answered struct {
		Me struct {
			Role string `json:"role"`
		} `json:"me"`
	}
	client.MustPost(`{ me { role } }`, &answered)

	if answered.Me.Role != role.Admin.String() {
		t.Errorf("role = %q, want %q", answered.Me.Role, role.Admin.String())
	}
}

func TestInviteStartsAtTheNarrowestRoleWhenTheInputNamesNone(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		Invite struct {
			Delivered bool `json:"delivered"`
		} `json:"invite"`
	}
	client.MustPost(`mutation { invite(`+
		`email: "maria@example.com", name: "Maria Perez"`+
		`) { delivered } }`, &answered)

	if got := invitedRole(t, store, "maria@example.com"); got != role.Member.String() {
		t.Errorf("role = %q, want %q, an account starts at the narrowest role",
			got, role.Member.String())
	}
}

// invitedRole answers the role the store holds for one invited address.
func invitedRole(t *testing.T, store *testkit.Store, email string) string {
	t.Helper()
	held, err := store.UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("UserByEmail() error = %v, want the invited account", err)
	}
	return held.Role
}

func TestInviteStartsAtTheRoleTheInputNames(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		Invite struct {
			Delivered bool `json:"delivered"`
		} `json:"invite"`
	}
	client.MustPost(`mutation { invite(`+
		`email: "maria@example.com", name: "Maria Perez", role: "admin"`+
		`) { delivered } }`, &answered)

	if got := invitedRole(t, store, "maria@example.com"); got != role.Admin.String() {
		t.Errorf("role = %q, want %q", got, role.Admin.String())
	}
}

func TestInviteRefusesARoleTheRegistryDoesNotKnow(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(`mutation { invite(` +
		`email: "maria@example.com", name: "Maria Perez", role: "root"` +
		`) { delivered } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, answered.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
	wantUnknownRole(t, answered.Errors)
}

func TestInviteRefusesARoleBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(`mutation { invite(` +
		`email: "maria@example.com", name: "Maria Perez", ` +
		`role: "` + stewardRole.String() + `") { delivered } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("invite answered no error, want an admin refused a role it does not hold")
	}
}

func TestMeAnswersTheCapabilitiesTheRoleHolds(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, role.Admin)
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	var answered struct {
		Me struct {
			Capabilities []string `json:"capabilities"`
			Grantable    []string `json:"grantable"`
		} `json:"me"`
	}
	client.MustPost(`{ me { capabilities grantable } }`, &answered)

	want := []string{role.ManageUsers, role.ManageWebhooks}
	if !slices.Equal(answered.Me.Capabilities, want) {
		t.Errorf("capabilities = %v, want the admin's %v", answered.Me.Capabilities, want)
	}
	if !slices.Contains(answered.Me.Grantable, role.Admin.String()) {
		t.Errorf("grantable = %v, want an admin able to grant admin", answered.Me.Grantable)
	}
	if slices.Contains(answered.Me.Grantable, stewardRole.String()) {
		t.Errorf("grantable = %v, want a role holding more kept out of reach", answered.Me.Grantable)
	}
}

func TestMeAnswersNothingForAMember(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, role.Member)
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	var answered struct {
		Me struct {
			Capabilities []string `json:"capabilities"`
			Grantable    []string `json:"grantable"`
		} `json:"me"`
	}
	client.MustPost(`{ me { capabilities grantable } }`, &answered)

	if len(answered.Me.Capabilities) != 0 {
		t.Errorf("capabilities = %v, want none for a member", answered.Me.Capabilities)
	}
	if !slices.Equal(answered.Me.Grantable, []string{role.Member.String()}) {
		t.Errorf("grantable = %v, want a member able to grant only its own role", answered.Me.Grantable)
	}
}

func TestMeAnswersNothingForAnAccountHoldingNoRole(t *testing.T) {
	t.Parallel()

	resolver, held := newRoledResolver(t, "")
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	var answered struct {
		Me struct {
			Capabilities []string `json:"capabilities"`
			Grantable    []string `json:"grantable"`
		} `json:"me"`
	}
	client.MustPost(`{ me { capabilities grantable } }`, &answered)

	if len(answered.Me.Capabilities) != 0 {
		t.Errorf("capabilities = %v, want none for an account holding no role", answered.Me.Capabilities)
	}
	if !slices.Equal(answered.Me.Grantable, []string{role.Member.String()}) {
		t.Errorf("grantable = %v, want only the role holding nothing", answered.Me.Grantable)
	}
}

func TestSetUserRoleStandsAUserInTheTierItNames(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		SetUserRole bool `json:"setUserRole"`
	}
	client.MustPost(settingRole(held.ID, role.Admin.String()), &answered)

	if !answered.SetUserRole {
		t.Error("setUserRole answered false, want the change reported")
	}
	if got := store.Users[held.ID].Role; got != role.Admin.String() {
		t.Errorf("stored role = %q, want %q", got, role.Admin.String())
	}
}

func TestSetUserRoleRefusesATierNoDeploymentKnows(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(held.ID, "root"))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, answered.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
	wantUnknownRole(t, answered.Errors)
	if got := store.Users[held.ID].Role; got != role.Member.String() {
		t.Errorf("stored role = %q, want the refused write to store nothing", got)
	}
}

func TestSetUserRoleRefusesGrantingBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(held.ID, "steward"))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserRole answered no error, want an admin refused a role it does not hold")
	}
	if got := store.Users[held.ID].Role; got != role.Member.String() {
		t.Errorf("stored role = %q, want the refused write to store nothing", got)
	}
}

func TestSetUserRoleRefusesTouchingAnAccountBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, "steward")
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(held.ID, role.Member.String()))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserRole answered no error, want an admin refused an account holding more")
	}
	if got := store.Users[held.ID].Role; got != "steward" {
		t.Errorf("stored role = %q, want the refused write to leave it", got)
	}
}

func TestAnAdminReachesAnAccountWhoseRoleNobodyDeclares(t *testing.T) {
	t.Parallel()

	movingToMember := func(id uuid.UUID) string { return settingRole(id, role.Member.String()) }
	holdsMember := func(account gouncer.User) bool { return account.Role == role.Member.String() }
	barring := func(id uuid.UUID) string { return settingDisabled(id, true) }
	barred := func(account gouncer.User) bool { return account.Disabled }
	cases := []struct {
		name     string
		stored   role.Role
		mutation func(uuid.UUID) string
		applied  func(gouncer.User) bool
	}{
		{"setUserRole on an account holding no role", "", movingToMember, holdsMember},
		{"setUserRole on an account holding a retired role", retiredRole, movingToMember, holdsMember},
		{"setUserDisabled on an account holding a retired role", retiredRole, barring, barred},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, held := roledStore(t, tc.stored)
			actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
			client := newActingClient(t, newAuthResolver(store), actor)

			answered, err := client.RawPost(tc.mutation(held.ID))

			if err != nil {
				t.Fatalf("RawPost() error = %v, want nil", err)
			}
			if len(answered.Errors) != 0 {
				t.Errorf("errors = %s, want an admin able to write the account", answered.Errors)
			}
			if got := store.Users[held.ID]; !tc.applied(got) {
				t.Errorf("stored role = %q, disabled = %t, want the write applied", got.Role, got.Disabled)
			}
		})
	}
}

func TestAWriteRefusesAnAccountWhoseRoleChangedAfterTheCheck(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		mutation func(uuid.UUID) string
	}{
		{"setUserRole from member to admin", func(id uuid.UUID) string { return settingRole(id, role.Admin.String()) }},
		{"setUserDisabled barring the member", func(id uuid.UUID) string { return settingDisabled(id, true) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, held := roledStore(t, role.Member)
			acting := roledUser(t, store, "admin@example.com", role.Admin)
			resolver := newAuthResolver(store)
			moving := roleChangingStore{Store: store, id: held.ID, to: stewardRole.String()}
			resolver.Admin = adminOver(moving, resolver.Roles)
			client := newActingClient(t, resolver, authkit.Identity{ID: acting, Role: role.Admin.String()})

			answered, err := client.RawPost(tc.mutation(held.ID))

			if err != nil {
				t.Fatalf("RawPost() error = %v, want nil", err)
			}
			if got := firstErrorCode(t, answered.Errors); got != "CONFLICT" {
				t.Errorf("code = %q, want CONFLICT", got)
			}
			if got := firstErrorReason(t, answered.Errors); got != "role_changed" {
				t.Errorf("reason = %q, want role_changed", got)
			}
			if got := store.Users[held.ID]; got.Role != stewardRole.String() || got.Disabled {
				t.Errorf("stored role = %q, disabled = %t, want the moved role kept and the account enabled",
					got.Role, got.Disabled)
			}
		})
	}
}

func TestSetUserDisabledRefusesEnablingAnAccountWhoseRoleNobodyDeclares(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		stored role.Role
	}{
		{"an account holding no role", ""},
		{"an account holding a retired role", retiredRole},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			store, held := roledStore(t, tc.stored)
			held.Disabled = true
			store.Users[held.ID] = held
			actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
			client := newActingClient(t, newAuthResolver(store), actor)

			answered, err := client.RawPost(settingDisabled(held.ID, false))

			if err != nil {
				t.Fatalf("RawPost() error = %v, want nil", err)
			}
			wantBeyondReach(t, answered.Errors)
			if !store.Users[held.ID].Disabled {
				t.Error("the account is enabled, want the refused write to leave it disabled")
			}
		})
	}
}

func TestSetUserDisabledRefusesBarringItself(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	answered, err := client.RawPost(settingDisabled(held.ID, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorReason(t, answered.Errors); got != "self_disable_refused" {
		t.Errorf("reason = %q, want self_disable_refused", got)
	}
	if store.Users[held.ID].Disabled {
		t.Error("the account is barred, want the refused write to leave it enabled")
	}
}

func TestSetUserDisabledBarsThroughTheAccountSeam(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		SetUserDisabled bool `json:"setUserDisabled"`
	}
	client.MustPost(settingDisabled(held.ID, true), &answered)

	if !answered.SetUserDisabled {
		t.Error("setUserDisabled answered false, want the change reported")
	}
	if !store.Users[held.ID].Disabled {
		t.Error("the account is enabled, want the guarded write to bar it")
	}
}

func TestSetUserDisabledRefusesAnAccountBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, "steward")
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(held.ID, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error, want an admin refused an account holding more")
	}
	if store.Users[held.ID].Disabled {
		t.Error("the account is barred, want the refused write to leave it")
	}
}

func TestSetUserRoleReportsAStoreThatCannotList(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	store.ListUsersErr = errListing
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(held.ID, role.Admin.String()))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserRole answered no error while the store refuses to list, want one")
	}
}

func TestSetUserRoleReportsAStoreThatCannotWrite(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	store.SetRoleErr = errWriting
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(held.ID, role.Admin.String()))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserRole answered no error while the store refuses the write, want one")
	}
}

func TestSetUserDisabledReportsAStoreThatCannotList(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	store.ListUsersErr = errListing
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(held.ID, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error while the store refuses to list, want one")
	}
}

func TestSetUserDisabledAdmitsAnAccountItBarred(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	held.Disabled = true
	store.Users[held.ID] = held
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		SetUserDisabled bool `json:"setUserDisabled"`
	}
	client.MustPost(settingDisabled(held.ID, false), &answered)

	if !answered.SetUserDisabled {
		t.Error("setUserDisabled answered false, want the change reported")
	}
	if store.Users[held.ID].Disabled {
		t.Error("the account is still barred, want it admitted")
	}
}

func TestSetUserDisabledReportsAStoreThatCannotWrite(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Member)
	store.SetDisabledErr = errWriting
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingDisabled(held.ID, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error while the store refuses the write, want one")
	}
}

func TestSetUserRoleReportsAnAccountNobodyHolds(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Member)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(settingRole(uuid.Must(uuid.NewV7()), role.Admin.String()))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserRole answered no error, want an account nobody holds refused")
	}
}
