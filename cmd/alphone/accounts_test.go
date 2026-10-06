// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	accounts "github.com/gopherium/framework/gonsole/auth"
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
		{Name: "steward", Capabilities: []string{string(role.ManageUsers), string(role.ManageWebhooks)}},
	}}}, nil
}

// accountsDatabase returns the settings of a database the command line migrated, holding an admin and a member.
func accountsDatabase(t *testing.T) map[string]string {
	t.Helper()
	env := map[string]string{"ALPHONE_DATABASE_URL": testDatabaseURL(t)}
	if got := testkit.Run(t, bareProgram(env), "", "migrate"); got.Code != gonsole.ExitDone {
		t.Fatalf("migrate = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	createAccount(t, testGetenv(env), "admin@example.com", role.Admin.String())
	createAccount(t, testGetenv(env), "maria.perez@example.com", role.Member.String())
	return env
}

// recordsOf returns the lines account:records lists over env.
func recordsOf(t *testing.T, env map[string]string) []string {
	t.Helper()
	got := testkit.Run(t, bareProgram(env), "", "account:records")
	if got.Code != gonsole.ExitDone {
		t.Fatalf("account:records = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	return strings.FieldsFunc(got.Stdout, func(r rune) bool { return r == '\n' })
}

// unchangedMember checks that maria.perez@example.com is still an enabled member and that no change is on record.
func unchangedMember(t *testing.T, env map[string]string) {
	t.Helper()
	held := accountAt(t, env["ALPHONE_DATABASE_URL"], "maria.perez@example.com")
	if held.Role != role.Member.String() || held.Disabled {
		t.Errorf("maria.perez@example.com holds %q with disabled %v, want the member role kept and enabled",
			held.Role, held.Disabled)
	}
	if records := recordsOf(t, env); len(records) != 0 {
		t.Errorf("records = %q, want none", records)
	}
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
		"account:grant-role":   {"-role", "admin", "-yes", "-as", "admin@example.com"},
		"account:role":         {"maria.perez@example.com", "admin", "-yes", "-as", "admin@example.com"},
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

	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, refused), "",
		"account:grant-role", "-role", "admin", "-as", "admin@example.com")

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
		"account:grant-role": {"account:grant-role", "-role", "undeclared", "-yes", "-as", "admin@example.com"},
	}
	for name, args := range lines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := accountsDatabase(t)
			databaseURL := env["ALPHONE_DATABASE_URL"]
			storeRoleless(t, databaseURL, "none@example.com")

			got := testkit.Run(t, bareProgram(env), typedPassword+"\n", args...)

			want := "alphone: unknown role \"undeclared\", want admin or member\n"
			if got.Code != gonsole.ExitMisused || !strings.Contains(got.Stderr, want) {
				t.Errorf("%s = %d with stderr %q, want 2 and %q", name, got.Code, got.Stderr, want)
			}
			if accounts := countRows(t, testPool(t, databaseURL), "auth.users"); accounts != 3 {
				t.Errorf("accounts = %d, want only the three held, nothing created", accounts)
			}
			if held := roleOf(t, databaseURL, "none@example.com"); held != "" {
				t.Errorf("none@example.com holds %q, want no role written", held)
			}
		})
	}
}

func TestAccountChangesWantTheActingAccount(t *testing.T) {
	t.Parallel()

	lines := map[string][]string{
		"account:role":       {"maria.perez@example.com", "admin", "-yes"},
		"account:disable":    {"maria.perez@example.com", "-yes"},
		"account:enable":     {"maria.perez@example.com", "-yes"},
		"account:grant-role": {"-role", "admin", "-yes"},
	}
	for name, args := range lines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := accountsDatabase(t)

			got := testkit.Run(t, bareProgram(env), "", append([]string{name}, args...)...)

			want := "alphone: " + name + " wants -as <email>\n"
			if got.Code != gonsole.ExitMisused || got.Stdout != "" || !strings.HasPrefix(got.Stderr, want) {
				t.Errorf("%s = %d, stdout %q, stderr %q, want 2 and %q", name, got.Code, got.Stdout, got.Stderr, want)
			}
			unchangedMember(t, env)
		})
	}
}

func TestAGuardedCommandOnADatabaseWithoutTheRecordsPointsAtMigrate(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		database func(*testing.T) string
		schemas  []string
	}{
		"a bare database":                        {barePostgres, []string{}},
		"a database an earlier release migrated": {testDatabaseURL, []string{"auth", "core"}},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			databaseURL := tc.database(t)

			got := testkit.Run(t, bareProgram(map[string]string{"ALPHONE_DATABASE_URL": databaseURL}), "",
				"account:grant-role", "-role", "member", "-yes", "-as", "admin@example.com")

			want := "alphone: the command records are missing, run migrate first\n"
			if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
				t.Errorf("account:grant-role = %d, stdout %q, stderr %q, want 1 and %q",
					got.Code, got.Stdout, got.Stderr, want)
			}
			if schemas := extraSchemas(t, databaseURL); !slices.Equal(schemas, tc.schemas) {
				t.Errorf("the database holds the schemas %v, want %v, no schema step applied", schemas, tc.schemas)
			}
		})
	}
}

func TestAMemberCannotChangeTheRoleOfAnAccount(t *testing.T) {
	t.Parallel()

	env := accountsDatabase(t)

	got := testkit.Run(t, bareProgram(env), "",
		"account:role", "maria.perez@example.com", "admin", "-yes", "-as", "maria.perez@example.com")

	want := "alphone: the account maria.perez@example.com holds the role member, which lacks manage_users\n"
	if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
		t.Errorf("account:role as a member = %d, stdout %q, stderr %q, want 1 and %q",
			got.Code, got.Stdout, got.Stderr, want)
	}
	unchangedMember(t, env)
}

func TestAnAdminsRoleChangeIsRecorded(t *testing.T) {
	t.Parallel()

	env := accountsDatabase(t)

	got := testkit.Run(t, bareProgram(env), "",
		"account:role", "maria.perez@example.com", "admin", "-yes", "-as", "admin@example.com")

	if got.Code != gonsole.ExitDone || got.Stdout != "set maria.perez@example.com to admin\n" {
		t.Fatalf("account:role as an admin = %d, stdout %q, stderr %q, want 0 and the change applied",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, env["ALPHONE_DATABASE_URL"], "maria.perez@example.com"); held != role.Admin.String() {
		t.Errorf("maria.perez@example.com holds %q, want %q", held, role.Admin.String())
	}
	held := recordsOf(t, env)
	want := "admin@example.com  account:role  maria.perez@example.com admin"
	if len(held) != 1 || !strings.Contains(held[0], want) {
		t.Errorf("records = %q, want the one change admin@example.com applied, %q", held, want)
	}
}

func TestAPreviewOfAnAccountChangeRecordsNothing(t *testing.T) {
	t.Parallel()

	env := accountsDatabase(t)

	got := testkit.Run(t, bareProgram(env), "",
		"account:role", "maria.perez@example.com", "admin", "-as", "admin@example.com")

	if got.Code != gonsole.ExitDone || got.Stdout != "would set maria.perez@example.com to admin\n" ||
		got.Stderr != "alphone: dry run, nothing changed, pass -yes to apply\n" {
		t.Errorf("account:role preview = %d, stdout %q, stderr %q, want 0 and a dry run",
			got.Code, got.Stdout, got.Stderr)
	}
	unchangedMember(t, env)
}

func TestAnAdminCannotGiveARoleAboveItsOwn(t *testing.T) {
	t.Parallel()

	env := accountsDatabase(t)
	above := func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{rolePlugin{declared: []sdk.RoleDeclaration{
			{Name: "steward", Capabilities: []string{string(role.ManageUsers), "manage_reports"}},
		}}}, nil
	}

	got := testkit.Run(t, programOver(role.NewRegistry(), testGetenv(env), above), "",
		"account:role", "maria.perez@example.com", "steward", "-yes", "-as", "admin@example.com")

	want := "alphone: the role steward carries manage_reports, which the account admin@example.com lacks\n"
	if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
		t.Errorf("account:role -role steward = %d, stdout %q, stderr %q, want 1 and %q",
			got.Code, got.Stdout, got.Stderr, want)
	}
	unchangedMember(t, env)
}

func TestAnActingAccountCannotChangeItsOwnRoleOrDisableItself(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
		want string
	}{
		"a role change":         {[]string{"account:role", "admin@example.com", "member", "-yes"}, "change its own role"},
		"a role change preview": {[]string{"account:role", "admin@example.com", "member"}, "change its own role"},
		"a disable":             {[]string{"account:disable", "admin@example.com", "-yes"}, "disable itself"},
		"a disable preview":     {[]string{"account:disable", "admin@example.com"}, "disable itself"},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			env := accountsDatabase(t)

			got := testkit.Run(t, bareProgram(env), "", slices.Concat(tc.args, []string{"-as", "admin@example.com"})...)

			want := "alphone: the account admin@example.com cannot " + tc.want + "\n"
			if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
				t.Errorf("%q = %d, stdout %q, stderr %q, want 1 and %q", tc.args, got.Code, got.Stdout, got.Stderr, want)
			}
			held := accountAt(t, env["ALPHONE_DATABASE_URL"], "admin@example.com")
			if held.Role != role.Admin.String() || held.Disabled {
				t.Errorf("admin@example.com holds %q with disabled %v, want admin kept and enabled", held.Role, held.Disabled)
			}
			unchangedMember(t, env)
		})
	}
}

func TestPaddedRecordSettingsReadLikePlainOnes(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value   string
		records int
	}{
		"ALPHONE_COMMAND_RECORD_TIMEOUT": {"  3s  ", 2},
		"ALPHONE_COMMAND_RECORDS_LIMIT":  {"  1  ", 1},
	}
	for key, tc := range tests {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			env := accountsDatabase(t)
			env[key] = tc.value

			if got := testkit.Run(t, bareProgram(env), "", "check"); got.Code != gonsole.ExitDone {
				t.Fatalf("check with %s=%q = %d with stderr %q, want 0", key, tc.value, got.Code, got.Stderr)
			}
			for _, held := range []string{role.Admin.String(), role.Member.String()} {
				got := testkit.Run(t, bareProgram(env), "",
					"account:role", "maria.perez@example.com", held, "-yes", "-as", "admin@example.com")
				if got.Code != gonsole.ExitDone {
					t.Fatalf("account:role %s with %s=%q = %d with stderr %q, want 0",
						held, key, tc.value, got.Code, got.Stderr)
				}
			}
			if records := recordsOf(t, env); len(records) != tc.records {
				t.Errorf("records with %s=%q = %q, want %d", key, tc.value, records, tc.records)
			}
		})
	}
}

func TestTheRoleTableCountsEveryRoleThatManagesUsersAsPrivileged(t *testing.T) {
	t.Parallel()

	registry := role.NewRegistry()
	var held accounts.Roles
	reading := gonsole.Program{
		Name:     "alphone",
		Env:      settingsEnv(testGetenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})),
		Database: "DATABASE_URL",
		Plugins:  loadPlugins(registry, stewardDeclaring),
		Commands: []gonsole.Command{{
			Name:    "roles",
			Summary: "read the role table of the account commands",
			Run: func(ctx context.Context, call gonsole.Call) (err error) {
				held, err = declaredRoles(registry)(ctx, call)
				return err
			},
		}},
	}

	if got := testkit.Run(t, reading, "", "roles"); got.Code != gonsole.ExitDone {
		t.Fatalf("roles = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if want := []string{role.Admin.String(), "steward"}; !slices.Equal(held.Privileged, want) {
		t.Errorf("privileged roles = %v, want %v, every role carrying manage_users", held.Privileged, want)
	}
	want := []string{string(role.ManageUsers), string(role.ManageWebhooks)}
	if !slices.Equal(held.Capabilities["steward"], want) {
		t.Errorf("the steward role carries %v, want %v", held.Capabilities["steward"], want)
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
		"account:role", "admin@example.com", "member", "-yes", "-as", "maria.perez@example.com")

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

	env := accountsDatabase(t)
	storeRoleless(t, env["ALPHONE_DATABASE_URL"], "none@example.com")

	got := testkit.Run(t, bareProgram(env), "",
		"account:grant-role", "-role", "member", "-yes", "-as", "admin@example.com")

	if got.Code != gonsole.ExitDone || got.Stdout != "granted member to 1 account\n" {
		t.Fatalf("account:grant-role = %d, stdout %q, stderr %q, want 0 and the one account counted",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, env["ALPHONE_DATABASE_URL"], "none@example.com"); held != role.Member.String() {
		t.Errorf("none@example.com holds %q, want %q", held, role.Member.String())
	}
}

func TestGrantRoleLeavesAnAccountThatHoldsOne(t *testing.T) {
	t.Parallel()

	env := accountsDatabase(t)
	storeRoleless(t, env["ALPHONE_DATABASE_URL"], "standing@example.com")
	first := testkit.Run(t, bareProgram(env), "",
		"account:grant-role", "-role", "admin", "-yes", "-as", "admin@example.com")
	if first.Code != gonsole.ExitDone {
		t.Fatalf("first account:grant-role = %d with stderr %q, want 0", first.Code, first.Stderr)
	}

	got := testkit.Run(t, bareProgram(env), "",
		"account:grant-role", "-role", "member", "-yes", "-as", "admin@example.com")

	if got.Code != gonsole.ExitDone || got.Stdout != "granted member to 0 accounts\n" {
		t.Fatalf("second account:grant-role = %d, stdout %q, stderr %q, want 0 and no account counted",
			got.Code, got.Stdout, got.Stderr)
	}
	if held := roleOf(t, env["ALPHONE_DATABASE_URL"], "standing@example.com"); held != role.Admin.String() {
		t.Errorf("standing@example.com holds %q, want %q, a second run leaves an account that holds one",
			held, role.Admin.String())
	}
}
