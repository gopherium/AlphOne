// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/gouncer"
	"github.com/gopherium/gouncer/authkit"
	"github.com/gopherium/gouncer/authkit/testkit"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/role"
)

// stewardRole is the plugin declared role standing above admin in these tests.
const stewardRole role.Role = "steward"

// errListing reports a store that cannot list the accounts.
var errListing = errors.New("accounts unavailable")

// errWriting reports a store that cannot store a role.
var errWriting = errors.New("role unwritable")

// init declares the plugin role once, since the registry a resolver reads is the deployment's.
func init() {
	if err := role.Grant(stewardRole, role.ManageUsers, "manage_reports"); err != nil {
		panic(err)
	}
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

func TestCreateUserStartsAtTheNarrowestRoleWhenTheInputNamesNone(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		CreateUser struct {
			Role string `json:"role"`
		} `json:"createUser"`
	}
	client.MustPost(`mutation { createUser(`+
		`email: "maria@example.com", name: "Maria Perez", password: "correct horse battery"`+
		`) { role } }`, &answered)

	if answered.CreateUser.Role != role.Member.String() {
		t.Errorf("role = %q, want %q, an account starts at the narrowest role",
			answered.CreateUser.Role, role.Member.String())
	}
}

func TestCreateUserStartsAtTheRoleTheInputNames(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	var answered struct {
		CreateUser struct {
			Role string `json:"role"`
		} `json:"createUser"`
	}
	client.MustPost(`mutation { createUser(`+
		`email: "maria@example.com", name: "Maria Perez", password: "correct horse battery", role: "admin"`+
		`) { role } }`, &answered)

	if answered.CreateUser.Role != role.Admin.String() {
		t.Errorf("role = %q, want %q", answered.CreateUser.Role, role.Admin.String())
	}
}

func TestCreateUserRefusesARoleTheRegistryDoesNotKnow(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(`mutation { createUser(` +
		`email: "maria@example.com", name: "Maria Perez", password: "correct horse battery", role: "root"` +
		`) { role } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, answered.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
}

func TestCreateUserRefusesARoleBeyondTheCallersReach(t *testing.T) {
	t.Parallel()

	store, _ := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	actor := authkit.Identity{ID: uuid.Must(uuid.NewV7()), Role: role.Admin.String()}
	client := newActingClient(t, resolver, actor)

	answered, err := client.RawPost(`mutation { createUser(` +
		`email: "maria@example.com", name: "Maria Perez", password: "correct horse battery", ` +
		`role: "` + stewardRole.String() + `") { role } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("createUser answered no error, want an admin refused a role it does not hold")
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

	if !slices.Equal(answered.Me.Capabilities, []string{string(role.ManageUsers)}) {
		t.Errorf("capabilities = %v, want the admin's manage_users", answered.Me.Capabilities)
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

func TestSetUserDisabledRefusesBarringItself(t *testing.T) {
	t.Parallel()

	store, held := roledStore(t, role.Admin)
	resolver := newAuthResolver(store)
	client := newActingClient(t, resolver, authkit.Identity{ID: held.ID, Role: held.Role})

	answered, err := client.RawPost(settingDisabled(held.ID, true))

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("setUserDisabled answered no error, want an account refused barring itself")
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
