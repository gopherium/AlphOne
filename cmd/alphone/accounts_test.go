// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// typedPassword is the password typed on standard input for every account a test creates.
const typedPassword = "correct horse battery"

// storeRoleless stores one account holding no role and returns it.
func storeRoleless(t *testing.T, databaseURL, email string) gouncer.User {
	t.Helper()
	held, err := gouncer.NewUser(email, "Maria Perez", typedPassword)
	if err != nil {
		t.Fatalf("gouncer.NewUser() error = %v, want nil", err)
	}
	if err := authkitpg.NewUserStore(testPool(t, databaseURL)).CreateUser(t.Context(), held); err != nil {
		t.Fatalf("CreateUser() error = %v, want nil", err)
	}
	return held
}

// createAccount creates the account at email under the role through account:create-admin over getenv.
func createAccount(t *testing.T, getenv func(string) string, email, held string) {
	t.Helper()
	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, registeringNothing), typedPassword+"\n",
		"account:create-admin", "-email", email, "-name", "Account Holder", "-role", held)
	if got.Code != gonsole.ExitDone {
		t.Fatalf("account:create-admin %s = %d with stderr %q, want 0", email, got.Code, got.Stderr)
	}
}

// accountAt returns the account at email in the database at databaseURL.
func accountAt(t *testing.T, databaseURL, email string) gouncer.User {
	t.Helper()
	held, err := authkitpg.NewUserStore(testPool(t, databaseURL)).UserByEmail(t.Context(), email)
	if err != nil {
		t.Fatalf("UserByEmail(%s) error = %v, want the account", email, err)
	}
	return held
}

// roleOf returns the role the account at email holds in the database at databaseURL.
func roleOf(t *testing.T, databaseURL, email string) string {
	t.Helper()
	return accountAt(t, databaseURL, email).Role
}

// stewardDeclaring registers one plugin declaring the steward role.
func stewardDeclaring(sdk.Deps) ([]sdk.Plugin, error) {
	return []sdk.Plugin{rolePlugin{declared: []sdk.RoleDeclaration{
		{Name: "steward", Capabilities: []string{string(role.ManageUsers)}},
	}}}, nil
}

func TestCreateAdminTakesARoleAPluginDeclares(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, stewardDeclaring), typedPassword+"\n",
		"account:create-admin", "-email", "admin@example.com", "-name", "Account Holder", "-role", "steward")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("account:create-admin -role steward = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if held := roleOf(t, databaseURL, "admin@example.com"); held != "steward" {
		t.Errorf("admin@example.com holds %q, want the steward role the plugin declares", held)
	}
}

func TestEveryRoleWritingCommandRefusesAPluginItCannotRegister(t *testing.T) {
	t.Parallel()

	failing := func(sdk.Deps) ([]sdk.Plugin, error) { return nil, errPluginMigrate }
	tests := map[string][]string{
		"account:create-admin": {"-email", "admin@example.com", "-name", "Account Holder", "-role", "admin"},
		"account:grant-role":   {"-role", "admin", "-yes"},
		"account:role":         {"maria.perez@example.com", "admin", "-yes"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			databaseURL := testDatabaseURL(t)
			storeRoleless(t, databaseURL, "maria.perez@example.com")
			getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})

			got := testkit.Run(t, programOver(role.NewRegistry(), getenv, failing), typedPassword+"\n",
				append([]string{name}, args...)...)

			want := "register plugins: " + errPluginMigrate.Error()
			if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, want) {
				t.Errorf("%s = %d with stderr %q, want 1 and %q", name, got.Code, got.Stderr, want)
			}
			if accounts := countRows(t, testPool(t, databaseURL), "auth.users"); accounts != 1 {
				t.Errorf("accounts = %d, want only the stored one, nothing created", accounts)
			}
			if held := roleOf(t, databaseURL, "maria.perez@example.com"); held != "" {
				t.Errorf("maria.perez@example.com holds %q, want no role written", held)
			}
		})
	}
}

func TestAccountCommandsRefuseARoleAPluginCannotDeclare(t *testing.T) {
	t.Parallel()

	refused := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{rolePlugin{declared: []sdk.RoleDeclaration{{Name: ""}}}}, nil
	}
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, refused), "", "account:grant-role", "-role", "admin")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, role.ErrEmptyRole.Error()) {
		t.Errorf("account:grant-role = %d with stderr %q, want 1 and the refused declaration named", got.Code, got.Stderr)
	}
}

func TestAccountCommandsRefuseARoleNothingDeclares(t *testing.T) {
	t.Parallel()

	lines := map[string][]string{
		"account:create-admin": {
			"account:create-admin", "-email", "admin@example.com", "-name", "Account Holder", "-role", "undeclared",
		},
		"account:grant-role": {"account:grant-role", "-role", "undeclared", "-yes"},
	}
	for name, args := range lines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			databaseURL := testDatabaseURL(t)
			storeRoleless(t, databaseURL, "maria.perez@example.com")

			got := testkit.Run(t, bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}),
				typedPassword+"\n", args...)

			want := "alphone: unknown role \"undeclared\", want admin or member\n"
			if got.Code != gonsole.ExitMisused || !strings.Contains(got.Stderr, want) {
				t.Errorf("%s = %d with stderr %q, want 2 and %q", name, got.Code, got.Stderr, want)
			}
			if accounts := countRows(t, testPool(t, databaseURL), "auth.users"); accounts != 1 {
				t.Errorf("accounts = %d, want only the stored one, nothing created", accounts)
			}
			if held := roleOf(t, databaseURL, "maria.perez@example.com"); held != "" {
				t.Errorf("maria.perez@example.com holds %q, want no role written", held)
			}
		})
	}
}

func TestTheLastPrivilegedAccountKeepsItsRoleAndStaysEnabled(t *testing.T) {
	t.Parallel()

	lines := map[string][]string{
		"a role change": {"account:role", "admin@example.com", "member", "-yes"},
		"a disable":     {"account:disable", "admin@example.com", "-yes"},
	}
	for testName, args := range lines {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			databaseURL := testDatabaseURL(t)
			env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}
			createAccount(t, testGetenv(env), "admin@example.com", role.Admin.String())

			got := testkit.Run(t, bareProgram(env), "", args...)

			want := "alphone: admin@example.com is the last enabled privileged account\n"
			if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
				t.Errorf("%q = %d, stdout %q, stderr %q, want 1 and %q", args, got.Code, got.Stdout, got.Stderr, want)
			}
			if held := accountAt(t, databaseURL, "admin@example.com"); held.Role != role.Admin.String() || held.Disabled {
				t.Errorf("admin@example.com holds %q with disabled %v, want admin kept and enabled", held.Role, held.Disabled)
			}
		})
	}
}

func TestAPluginRoleThatManagesUsersCoversTheLastAdministrator(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	createAccount(t, getenv, "admin@example.com", role.Admin.String())
	steward := testkit.Run(t, programOver(role.NewRegistry(), getenv, stewardDeclaring), typedPassword+"\n",
		"account:create-admin", "-email", "maria.perez@example.com", "-name", "Account Holder", "-role", "steward")
	if steward.Code != gonsole.ExitDone {
		t.Fatalf("account:create-admin -role steward = %d with stderr %q, want 0", steward.Code, steward.Stderr)
	}

	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, stewardDeclaring), "",
		"account:role", "admin@example.com", "member", "-yes")

	if got.Code != gonsole.ExitDone || got.Stdout != "set admin@example.com to member\n" {
		t.Errorf("account:role = %d, stdout %q, stderr %q, want 0 with the steward left to manage users",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, databaseURL, "admin@example.com"); held != role.Member.String() {
		t.Errorf("admin@example.com holds %q, want %q", held, role.Member.String())
	}
}

func TestGrantRoleReachesEveryAccountHoldingNone(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	storeRoleless(t, databaseURL, "none@example.com")

	got := testkit.Run(t, bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "",
		"account:grant-role", "-role", "member", "-yes")

	if got.Code != gonsole.ExitDone || got.Stdout != "granted member to 1 account\n" {
		t.Fatalf("account:grant-role = %d, stdout %q, stderr %q, want 0 and the one account counted",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, databaseURL, "none@example.com"); held != role.Member.String() {
		t.Errorf("none@example.com holds %q, want %q", held, role.Member.String())
	}
}

func TestGrantRoleLeavesAnAccountThatHoldsOne(t *testing.T) {
	t.Parallel()

	databaseURL := testDatabaseURL(t)
	storeRoleless(t, databaseURL, "standing@example.com")
	commands := bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL})
	first := testkit.Run(t, commands, "", "account:grant-role", "-role", "admin", "-yes")
	if first.Code != gonsole.ExitDone {
		t.Fatalf("first account:grant-role = %d with stderr %q, want 0", first.Code, first.Stderr)
	}

	got := testkit.Run(t, commands, "", "account:grant-role", "-role", "member", "-yes")

	if got.Code != gonsole.ExitDone || got.Stdout != "granted member to 0 accounts\n" {
		t.Fatalf("second account:grant-role = %d, stdout %q, stderr %q, want 0 and no account counted",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, databaseURL, "standing@example.com"); held != role.Admin.String() {
		t.Errorf("standing@example.com holds %q, want %q, a second run leaves an account that holds one",
			held, role.Admin.String())
	}
}
