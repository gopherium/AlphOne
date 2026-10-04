// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"
	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
)

// seedTokenUser provisions the account the token commands act for.
func seedTokenUser(t *testing.T, getenv func(string) string) {
	t.Helper()
	createAccount(t, getenv, "admin@example.com", "admin")
}

// tokenDatabase returns the address and the settings of a fresh migrated database holding admin@example.com.
func tokenDatabase(t *testing.T) (string, map[string]string) {
	t.Helper()
	databaseURL := testDatabaseURL(t)
	env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}
	seedTokenUser(t, testGetenv(env))
	return databaseURL, env
}

// mint runs token:create for admin@example.com over env with the extra flags, failing the test unless it succeeds.
func mint(t *testing.T, env map[string]string, extra ...string) testkit.Result {
	t.Helper()
	args := append([]string{"token:create", "-email", "admin@example.com", "-name", "n8n"}, extra...)
	got := testkit.Run(t, bareProgram(env), "", args...)
	if got.Code != gonsole.ExitDone {
		t.Fatalf("token:create = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	return got
}

// secretOf returns the token secret printed in output.
func secretOf(t *testing.T, output string) string {
	t.Helper()
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, apitoken.Prefix) {
			return field
		}
	}
	t.Fatalf("output = %q, want it to carry a secret", output)
	return ""
}

// storedToken returns the token persisted under the given secret.
func storedToken(t *testing.T, databaseURL, secret string) apitoken.Token {
	t.Helper()
	stored, err := postgres.NewTokenStore(testPool(t, databaseURL)).ByHash(t.Context(), apitoken.HashSecret(secret))
	if err != nil {
		t.Fatalf("ByHash() error = %v, want the stored token", err)
	}
	return stored
}

func TestTokenCreatePrintsTheSecretOnce(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)

	got := mint(t, env)

	secret := secretOf(t, got.Stdout)
	stored := storedToken(t, databaseURL, secret)
	want := fmt.Sprintf("created token %s\nsecret: %s\nstore it now, it is never shown again\nscopes %s, expires %s\n",
		stored.ID, secret, stored.Scopes, stored.ExpiresAt.UTC().Format(dateLayout))
	if got.Stdout != want || got.Stderr != "" {
		t.Errorf("token:create stdout %q, stderr %q, want %q and nothing on stderr", got.Stdout, got.Stderr, want)
	}
	if stored.Name != "n8n" {
		t.Errorf("stored name = %q, want %q", stored.Name, "n8n")
	}
}

func TestTokenCreateGrantsFullScopeForNinetyDaysAndSaysSo(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)

	got := mint(t, env)

	stored := storedToken(t, databaseURL, secretOf(t, got.Stdout))
	if got, want := stored.Scopes.String(), apitoken.Wildcard; got != want {
		t.Errorf("scopes = %q, want %q", got, want)
	}
	if want := stored.CreatedAt.Add(defaultTokenLifetime); !stored.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", stored.ExpiresAt, want)
	}
	said := fmt.Sprintf("scopes %s, expires %s", stored.Scopes, stored.ExpiresAt.UTC().Format(dateLayout))
	if !strings.Contains(got.Stdout, said) {
		t.Errorf("output = %q, want it to carry %q", got.Stdout, said)
	}
}

func TestTokenListShowsTheScopesAndTheExpiry(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	stored := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("token:list = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if want := "scopes " + stored.Scopes.String(); !strings.Contains(got.Stdout, want) {
		t.Errorf("output = %q, want it to carry %q", got.Stdout, want)
	}
	if want := "expires " + stored.ExpiresAt.UTC().Format(dateLayout); !strings.Contains(got.Stdout, want) {
		t.Errorf("output = %q, want it to carry %q", got.Stdout, want)
	}
}

func TestTokenCreateGrantsOnlyTheScopesAsked(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)

	got := mint(t, env, "-scope", "tasks:write", "-scope", "contacts:read")

	stored := storedToken(t, databaseURL, secretOf(t, got.Stdout))
	if got, want := stored.Scopes.String(), "contacts:read tasks:write"; got != want {
		t.Errorf("scopes = %q, want %q", got, want)
	}
}

func TestTokenCreateRefusesATokenItCannotMint(t *testing.T) {
	t.Parallel()

	_, env := tokenDatabase(t)
	tests := map[string]struct {
		args []string
		want string
	}{
		"an area no schema declares": {
			[]string{"-email", "admin@example.com", "-name", "typo", "-scope", "contact:read"},
			apitoken.ErrUnknownArea.Error(),
		},
		"an unreadable lifetime": {
			[]string{"-email", "admin@example.com", "-name", "n8n", "-ttl", "soon"}, "parse ttl",
		},
		"a lifetime that would overflow": {
			[]string{"-email", "admin@example.com", "-name", "n8n", "-ttl", "213504"},
			apitoken.ErrLifetimeTooLong.Error(),
		},
		"a malformed scope": {
			[]string{"-email", "admin@example.com", "-name", "n8n", "-scope", "tasks:admin"},
			apitoken.ErrMalformedScope.Error(),
		},
		"an address nobody answers to": {
			[]string{"-email", "nobody@example.com", "-name", "n8n"}, gouncer.ErrUserNotFound.Error(),
		},
		"a blank name": {
			[]string{"-email", "admin@example.com", "-name", "  "}, apitoken.ErrEmptyName.Error(),
		},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(env), "", append([]string{"token:create"}, tc.args...)...)

			if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, tc.want) || got.Stdout != "" {
				t.Errorf("token:create %q = %d, stdout %q, stderr %q, want 1, nothing minted and %q",
					tc.args, got.Code, got.Stdout, got.Stderr, tc.want)
			}
		})
	}
}

func TestTokenCreateRefusesABlankOrSpacedScopeBeforeMinting(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	for _, scope := range []string{"", "  ", "contacts:read tasks:write"} {
		got := testkit.Run(t, bareProgram(env), "",
			"token:create", "-email", "admin@example.com", "-name", "n8n", "-scope", scope)

		want := fmt.Sprintf("alphone: token:create: invalid value %q for flag -scope: %v: %q\n",
			scope, apitoken.ErrMalformedScope, scope)
		if got.Code != gonsole.ExitMisused || got.Stdout != "" || !strings.HasPrefix(got.Stderr, want) {
			t.Errorf("token:create -scope %q = %d, stdout %q, stderr %q, want 2 and %q",
				scope, got.Code, got.Stdout, got.Stderr, want)
		}
		if minted := countRows(t, testPool(t, databaseURL), "core.api_tokens"); minted != 0 {
			t.Errorf("the database holds %d tokens after -scope %q, want none minted", minted, scope)
		}
	}
}

func TestTokenCreateLastsAsLongAsAsked(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)

	got := mint(t, env, "-ttl", "7")

	stored := storedToken(t, databaseURL, secretOf(t, got.Stdout))
	if want := stored.CreatedAt.Add(7 * 24 * time.Hour); !stored.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v", stored.ExpiresAt, want)
	}
}

func TestTokenCreateLivesForeverWhenAsked(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)

	got := mint(t, env, "-ttl", "never")

	stored := storedToken(t, databaseURL, secretOf(t, got.Stdout))
	if !stored.ExpiresAt.IsZero() {
		t.Errorf("expires at %v, want never", stored.ExpiresAt)
	}
	if !strings.Contains(got.Stdout, "expires never") {
		t.Errorf("output = %q, want it to say the token never expires", got.Stdout)
	}
}

func TestTokenListNamesEveryTokenWithoutItsSecret(t *testing.T) {
	t.Parallel()

	_, env := tokenDatabase(t)
	mint(t, env)

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("token:list = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	if !strings.Contains(got.Stdout, "  n8n  scopes ") {
		t.Errorf("output = %q, want it to name the token", got.Stdout)
	}
	if strings.Contains(got.Stdout, apitoken.Prefix) {
		t.Errorf("output = %q, want no secret in a listing", got.Stdout)
	}
}

func TestTokenRevokeRemovesTheToken(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	secret := secretOf(t, mint(t, env).Stdout)
	stored := storedToken(t, databaseURL, secret)

	got := testkit.Run(t, bareProgram(env), "",
		"token:revoke", "-email", "admin@example.com", "-id", stored.ID.String())

	if got.Code != gonsole.ExitDone || got.Stdout != "revoked token "+stored.ID.String()+"\n" {
		t.Fatalf("token:revoke = %d, stdout %q, stderr %q, want 0 and the token named",
			got.Code, got.Stdout, got.Stderr)
	}
	_, err := postgres.NewTokenStore(testPool(t, databaseURL)).ByHash(t.Context(), apitoken.HashSecret(secret))
	if err == nil {
		t.Error("ByHash() after token:revoke found the token, want it gone")
	}
}

func TestTokenRevokeRefusesAnUnreadableID(t *testing.T) {
	t.Parallel()

	_, env := tokenDatabase(t)

	got := testkit.Run(t, bareProgram(env), "", "token:revoke", "-email", "admin@example.com", "-id", "not-a-uuid")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, "parse token id") {
		t.Errorf("token:revoke -id not-a-uuid = %d with stderr %q, want 1 and the id refused", got.Code, got.Stderr)
	}
}

func TestTokenListShowsTheLastUseDate(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	stored := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))
	used := time.Now().UTC()
	if err := postgres.NewTokenStore(testPool(t, databaseURL)).TouchLastUsed(t.Context(), stored.ID, used); err != nil {
		t.Fatalf("TouchLastUsed() error = %v, want nil", err)
	}

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	want := "last used " + used.Format(dateLayout)
	if got.Code != gonsole.ExitDone || !strings.Contains(got.Stdout, want) {
		t.Errorf("token:list = %d with stdout %q, want 0 and %q", got.Code, got.Stdout, want)
	}
}

func TestTokenCommandsWantTheOwnersAddress(t *testing.T) {
	t.Parallel()

	lines := map[string][]string{
		"token:create with a blank -email": {"token:create", "-email", "  ", "-name", "n8n"},
		"token:create with no -email":      {"token:create", "-name", "n8n"},
		"token:list with a blank -email":   {"token:list", "-email", "  "},
		"token:list with no -email":        {"token:list"},
		"token:revoke with a blank -email": {"token:revoke", "-email", "  ", "-id", uuid.Nil.String()},
		"token:revoke with no -email":      {"token:revoke", "-id", uuid.Nil.String()},
	}
	for testName, args := range lines {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(nil), "", args...)

			want := "alphone: " + args[0] + " wants -email <address>\n"
			if got.Code != gonsole.ExitMisused || !strings.HasPrefix(got.Stderr, want) {
				t.Errorf("%q = %d with stderr %q, want 2 and %q", args, got.Code, got.Stderr, want)
			}
		})
	}
}

func TestTokenCommandsRefuseAFlagTheyDoNotDeclare(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "token:list", "-nope")

	want := "alphone: token:list: flag provided but not defined: -nope\n"
	if got.Code != gonsole.ExitMisused || !strings.HasPrefix(got.Stderr, want) {
		t.Errorf("token:list -nope = %d with stderr %q, want 2 and %q", got.Code, got.Stderr, want)
	}
}

func TestTokenCommandsRefuseAnUnparsableDatabaseAddress(t *testing.T) {
	t.Parallel()

	env := map[string]string{"ALPHONE_DATABASE_URL": "://not a url"}

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	if got.Code != gonsole.ExitFailed || !strings.Contains(got.Stderr, "parse database url") {
		t.Errorf("token:list = %d with stderr %q, want 1 and the malformed address named", got.Code, got.Stderr)
	}
}

func TestTokenCommandsReportAnUnreachableDatabase(t *testing.T) {
	t.Parallel()

	env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	if got.Code != gonsole.ExitFailed || got.Stderr == "" {
		t.Errorf("token:list = %d with stderr %q, want 1 and the unreachable database reported", got.Code, got.Stderr)
	}
}

func TestNoTokenCommandMigrates(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)
	env := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}
	lines := map[string][]string{
		"token:create": {"-email", "admin@example.com", "-name", "n8n"},
		"token:list":   {"-email", "admin@example.com"},
		"token:revoke": {"-email", "admin@example.com", "-id", uuid.Nil.String()},
	}
	for name, args := range lines {
		got := testkit.Run(t, bareProgram(env), "", append([]string{name}, args...)...)

		if got.Code != gonsole.ExitFailed {
			t.Errorf("%s on a bare database = %d with stderr %q, want 1 and no schema step", name, got.Code, got.Stderr)
		}
		if schemas := extraSchemas(t, databaseURL); len(schemas) > 0 {
			t.Errorf("the database holds the schemas %v after %s, want nothing migrated", schemas, name)
		}
	}
}

// closedTokenStore returns a token store whose pool is already closed.
func closedTokenStore(t *testing.T) *postgres.TokenStore {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	pool.Close()
	return postgres.NewTokenStore(pool)
}

func TestTokenStepsReportStoreFailures(t *testing.T) {
	t.Parallel()

	store := closedTokenStore(t)
	owner := uuid.Must(uuid.NewV7())
	call := gonsole.Call{Flags: map[string]string{"name": "n8n", "id": uuid.Nil.String()}, Stdout: io.Discard}
	steps := map[string]tokenStep{"token:create": createToken, "token:list": listTokens, "token:revoke": revokeToken}

	for name, step := range steps {
		if err := step(t.Context(), store, owner, call); err == nil {
			t.Errorf("the %s step on a closed pool error = nil, want the failure reported", name)
		}
	}
}

func TestTokenCreateHelpNamesEveryDeclaredArea(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "token:create", "-h")

	want := strings.Join([]string{
		"mint a token for one account and show its secret once",
		"",
		"Usage:",
		"  alphone token:create [flags]",
		"",
		"Flags:",
		"  -email address",
		"    \taddress of the account that owns the tokens",
		"  -name name",
		"    \tname of the token to create",
		"  -scope area:access",
		"    \tarea:access scope the token may act in, repeatable, the area one of " +
			strings.Join(graphres.DeclaredAreas(), ", "),
		"  -ttl days",
		"    \tdays the token lasts, or never",
		"",
	}, "\n")
	if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
		t.Errorf("token:create -h = %d, stdout %q, stderr %q, want 0 and its page\n%s",
			got.Code, got.Stdout, got.Stderr, want)
	}
}
