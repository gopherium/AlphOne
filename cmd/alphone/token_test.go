// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
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
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/tenant"
	"github.com/gopherium/alphone/sdk"
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

// placedMember creates maria.perez@example.com and stands it in a fresh tenant, answering the account and the tenant.
func placedMember(t *testing.T, databaseURL string, env map[string]string) (gouncer.User, uuid.UUID) {
	t.Helper()
	createAccount(t, testGetenv(env), "maria.perez@example.com", role.Member.String())
	member := accountAt(t, databaseURL, "maria.perez@example.com")
	pool := testPool(t, databaseURL)
	acme := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", acme, "Acme"); err != nil {
		t.Fatalf("storing the tenant: %v", err)
	}
	if _, err := pool.Exec(t.Context(),
		"INSERT INTO core.tenant_members (user_id, tenant_id) VALUES ($1, $2)", member.ID, acme); err != nil {
		t.Fatalf("placing the member: %v", err)
	}
	return member, acme
}

// heldTokenIDs returns the ids of the tokens of userID the token store lists in the tenant ctx stands in.
func heldTokenIDs(t *testing.T, ctx context.Context, databaseURL string, userID uuid.UUID) []uuid.UUID {
	t.Helper()
	held, err := postgres.NewTokenStore(testPool(t, databaseURL)).ListForUser(ctx, userID)
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want the tokens", err)
	}
	ids := make([]uuid.UUID, 0, len(held))
	for _, token := range held {
		ids = append(ids, token.ID)
	}
	return ids
}

// servedAPI serves the API in process over the database at databaseURL until the test ends, answering its address.
func servedAPI(t *testing.T, databaseURL string) string {
	t.Helper()
	addr := freeAddr(t)
	ctx, cancel := context.WithCancel(t.Context())
	runErr := make(chan error, 1)
	go func() {
		runErr <- run(ctx, testGetenv(map[string]string{
			"ALPHONE_DATABASE_URL": databaseURL,
			"ALPHONE_ADDR":         addr,
		}), io.Discard, registerPlugins)
	}()
	t.Cleanup(func() { stopRun(t, cancel, runErr) })
	waitForServer(t, "http://"+addr)
	return addr
}

// graphData posts query to the API at addr under credential, decodes its data into data and answers its cookies.
func graphData(t *testing.T, addr, query string, credential func(*http.Request), data any) []*http.Cookie {
	t.Helper()
	document, err := json.Marshal(map[string]string{"query": query})
	if err != nil {
		t.Fatalf("encoding the graph query: %v", err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+addr+"/api/graphql",
		strings.NewReader(string(document)))
	if err != nil {
		t.Fatalf("building the graph request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	credential(request)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("posting the graph request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	var answered struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	err = json.NewDecoder(response.Body).Decode(&answered)
	if err == nil && len(answered.Errors) == 0 {
		err = json.Unmarshal(answered.Data, data)
	}
	if err != nil || response.StatusCode != http.StatusOK || len(answered.Errors) > 0 {
		t.Fatalf("the API answered %q with %d, errors %s (%v), want it served",
			query, response.StatusCode, answered.Errors, err)
	}
	return response.Cookies()
}

// bearerTenant answers the id of the tenant the API at addr stands the bearer of secret in.
func bearerTenant(t *testing.T, addr, secret string) string {
	t.Helper()
	var answered struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
	}
	graphData(t, addr, "{ tenant { id } }", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+secret)
	}, &answered)
	return answered.Tenant.ID
}

// screenTokenIDs logs the account at email in to the API at addr and answers the ids of the tokens it lists.
func screenTokenIDs(t *testing.T, addr, email string) []string {
	t.Helper()
	login := fmt.Sprintf("mutation { login(email: %q, password: %q) { me { id } } }", email, typedPassword)
	var signedIn json.RawMessage
	cookies := graphData(t, addr, login, func(*http.Request) {}, &signedIn)
	var answered struct {
		APITokens []struct {
			ID string `json:"id"`
		} `json:"apiTokens"`
	}
	graphData(t, addr, "{ apiTokens { id } }", func(r *http.Request) {
		for _, cookie := range cookies {
			r.AddCookie(cookie)
		}
	}, &answered)
	ids := make([]string, 0, len(answered.APITokens))
	for _, listed := range answered.APITokens {
		ids = append(ids, listed.ID)
	}
	return ids
}

func TestTokenCreateStoresTheTokenInItsOwnersTenant(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	member, acme := placedMember(t, databaseURL, env)

	got := testkit.Run(t, bareProgram(env), "",
		"token:create", "-email", "maria.perez@example.com", "-name", "reporting")

	if got.Code != gonsole.ExitDone {
		t.Fatalf("token:create = %d with stderr %q, want 0", got.Code, got.Stderr)
	}
	secret := secretOf(t, got.Stdout)
	minted := storedToken(t, databaseURL, secret)
	inTenant := heldTokenIDs(t, sdk.WithTenant(t.Context(), acme), databaseURL, member.ID)
	if !slices.Contains(inTenant, minted.ID) {
		t.Errorf("the owner's tenant lists %v, want the minted token %v", inTenant, minted.ID)
	}
	if inDefault := heldTokenIDs(t, t.Context(), databaseURL, member.ID); slices.Contains(inDefault, minted.ID) {
		t.Errorf("the default tenant lists the minted token %v, want it absent", minted.ID)
	}
	addr := servedAPI(t, databaseURL)
	if standing := bearerTenant(t, addr, secret); standing != acme.String() {
		t.Errorf("the API stands the token in the tenant %s, want the owner's tenant %s", standing, acme)
	}
	if listed := screenTokenIDs(t, addr, "maria.perez@example.com"); !slices.Contains(listed, minted.ID.String()) {
		t.Errorf("the owner's screen lists %v, want the minted token %s", listed, minted.ID)
	}
}

func TestTokenListAndRevokeReachATokenInTheOwnersTenant(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	member, acme := placedMember(t, databaseURL, env)
	minted, err := apitoken.Mint(member.ID, "reporting", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	store := postgres.NewTokenStore(testPool(t, databaseURL))
	if err := store.Create(sdk.WithTenant(t.Context(), acme), minted.Token); err != nil {
		t.Fatalf("Create() error = %v, want the token stored in the owner's tenant", err)
	}

	listed := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "maria.perez@example.com")
	revoked := testkit.Run(t, bareProgram(env), "",
		"token:revoke", "-email", "maria.perez@example.com", "-id", minted.Token.ID.String(), "-yes")

	if want := minted.Token.ID.String() + "  reporting  scopes "; listed.Code != gonsole.ExitDone ||
		!strings.Contains(listed.Stdout, want) {
		t.Errorf("token:list = %d, stdout %q, stderr %q, want 0 and %q", listed.Code, listed.Stdout, listed.Stderr, want)
	}
	if revoked.Code != gonsole.ExitDone {
		t.Errorf("token:revoke -yes = %d with stderr %q, want 0", revoked.Code, revoked.Stderr)
	}
	if _, err := store.ByHash(t.Context(), minted.Token.Hash); !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() after token:revoke -yes error = %v, want the token gone", err)
	}
}

func TestTokenCommandsReportATenantTheyCannotRead(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	if _, err := testPool(t, databaseURL).Exec(t.Context(), "DROP TABLE core.tenant_members"); err != nil {
		t.Fatalf("dropping the tenant placements: %v", err)
	}

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com")

	if got.Code != gonsole.ExitFailed || got.Stdout != "" || !strings.Contains(got.Stderr, "read tenant") {
		t.Errorf("token:list = %d, stdout %q, stderr %q, want 1 and the tenant read failure named",
			got.Code, got.Stdout, got.Stderr)
	}
}

func TestTokenCommandsFindTheOwnerByAPaddedUpperCaseAddress(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	held := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))
	const typed = "  Admin@Example.COM "
	lines := map[string]struct {
		args []string
		want string
	}{
		"token:create": {[]string{"token:create", "-email", typed, "-name", "reporting"}, "created token "},
		"token:list":   {[]string{"token:list", "-email", typed}, held.ID.String() + "  n8n  scopes "},
		"token:revoke": {
			[]string{"token:revoke", "-email", typed, "-id", held.ID.String()},
			"would revoke token " + held.ID.String() + " (n8n) of admin@example.com\n",
		},
	}
	for name, line := range lines {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(env), "", line.args...)

			if got.Code != gonsole.ExitDone || !strings.Contains(got.Stdout, line.want) {
				t.Errorf("%s -email %q = %d, stdout %q, stderr %q, want 0 and %q",
					name, typed, got.Code, got.Stdout, got.Stderr, line.want)
			}
		})
	}
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
	if want := stored.CreatedAt.Add(90 * 24 * time.Hour); !stored.ExpiresAt.Equal(want) {
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

// helpPage returns the help page of the command called name.
func helpPage(t *testing.T, name string) string {
	t.Helper()
	got := testkit.Run(t, bareProgram(nil), "", name, "-h")
	if got.Code != gonsole.ExitDone {
		t.Fatalf("%s -h = %d with stderr %q, want 0 and its page", name, got.Code, got.Stderr)
	}
	return got.Stdout
}

func TestTokenCreateRefusesAValueItCannotReadBeforeTheDatabase(t *testing.T) {
	t.Parallel()

	env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}
	page := helpPage(t, "token:create")
	tests := map[string]struct {
		flag, value, reason string
	}{
		"an area no schema declares":        {"scope", "contact:read", `apitoken: unknown area: "contact"`},
		"a malformed scope":                 {"scope", "tasks:admin", `apitoken: malformed scope: "tasks:admin"`},
		"a lifetime in no whole days":       {"ttl", "soon", "want a whole number of days or never"},
		"a lifetime in a fraction of days":  {"ttl", "1.5", "want a whole number of days or never"},
		"a negative lifetime":               {"ttl", "-1", apitoken.ErrNegativeLifetime.Error()},
		"one day past the longest lifetime": {"ttl", "106752", apitoken.ErrLifetimeTooLong.Error() + ": 106752 days"},
		"a lifetime that would overflow":    {"ttl", "213504", apitoken.ErrLifetimeTooLong.Error() + ": 213504 days"},
		"more days than a number holds":     {"ttl", "99999999999999999999", apitoken.ErrLifetimeTooLong.Error()},
		"fewer days than a number holds":    {"ttl", "-99999999999999999999", apitoken.ErrNegativeLifetime.Error()},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(env), "",
				"token:create", "-email", "admin@example.com", "-name", "n8n", "-"+tc.flag, tc.value)

			want := fmt.Sprintf("alphone: token:create: invalid value %q for flag -%s: %s\n\n%s",
				tc.value, tc.flag, tc.reason, page)
			if got.Code != gonsole.ExitMisused || got.Stdout != "" || got.Stderr != want {
				t.Errorf("token:create -%s %q = %d, stdout %q, stderr %q, want 2, no database reached and %q",
					tc.flag, tc.value, got.Code, got.Stdout, got.Stderr, want)
			}
		})
	}
}

func TestTokenCreateRefusesAnAddressNobodyAnswersTo(t *testing.T) {
	t.Parallel()

	_, env := tokenDatabase(t)

	got := testkit.Run(t, bareProgram(env), "", "token:create", "-email", "nobody@example.com", "-name", "n8n")

	want := "alphone: " + gouncer.ErrUserNotFound.Error() + "\n"
	if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
		t.Errorf("token:create -email nobody@example.com = %d, stdout %q, stderr %q, want 1, nothing minted and %q",
			got.Code, got.Stdout, got.Stderr, want)
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

func TestTokenCreateTakesItsDefaultLifetimeFromTheSetting(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		setting string
		ttl     []string
		want    time.Duration
	}{
		"seven days":                       {"7", nil, 7 * 24 * time.Hour},
		"seven days padded with spaces":    {"  7  ", nil, 7 * 24 * time.Hour},
		"zero days, which never expire":    {"0", nil, apitoken.Never},
		"seven days and a -ttl of three":   {"7", []string{"-ttl", "3"}, 3 * 24 * time.Hour},
		"seven days and a -ttl of zero":    {"7", []string{"-ttl", "0"}, apitoken.Never},
		"the most days a token may last":   {"106751", nil, apitoken.MaxLifetimeDays * 24 * time.Hour},
		"a malformed setting under a -ttl": {"a season", []string{"-ttl", "3"}, 3 * 24 * time.Hour},
	}
	for testName, tc := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			databaseURL, env := tokenDatabase(t)
			env["ALPHONE_TOKEN_TTL_DAYS"] = tc.setting

			got := mint(t, env, tc.ttl...)

			stored := storedToken(t, databaseURL, secretOf(t, got.Stdout))
			lasts := apitoken.Never
			if !stored.ExpiresAt.IsZero() {
				lasts = stored.ExpiresAt.Sub(stored.CreatedAt)
			}
			if lasts != tc.want {
				t.Errorf("the token lasts %v (expires at %v), want %v", lasts, stored.ExpiresAt, tc.want)
			}
		})
	}
}

func TestTokenCreateRefusesAMalformedDefaultLifetime(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	env["ALPHONE_TOKEN_TTL_DAYS"] = "a season"

	got := testkit.Run(t, bareProgram(env), "", "token:create", "-email", "admin@example.com", "-name", "n8n")

	want := "alphone: ALPHONE_TOKEN_TTL_DAYS: must be a whole number, got \"a season\"\n"
	if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
		t.Errorf("token:create = %d, stdout %q, stderr %q, want 1 and %q", got.Code, got.Stdout, got.Stderr, want)
	}
	if minted := countRows(t, testPool(t, databaseURL), "core.api_tokens"); minted != 0 {
		t.Errorf("the database holds %d tokens, want none minted", minted)
	}
}

func TestCheckNamesAMalformedTokenLifetime(t *testing.T) {
	t.Parallel()

	reasons := map[string]string{
		"a season": "must be a whole number",
		"-1":       "must not be negative",
		"106752":   "must stand at or below 106751",
	}
	for value, reason := range reasons {
		t.Run(value, func(t *testing.T) {
			t.Parallel()

			env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL, "ALPHONE_TOKEN_TTL_DAYS": value}

			got := testkit.Run(t, bareProgram(env), "", "check")

			want := fmt.Sprintf("alphone: ALPHONE_TOKEN_TTL_DAYS: %s, got %q\n", reason, value)
			if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
				t.Errorf("check with ALPHONE_TOKEN_TTL_DAYS=%q = %d, stdout %q, stderr %q, want 1 and %q",
					value, got.Code, got.Stdout, got.Stderr, want)
			}
		})
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
		"token:revoke", "-email", "admin@example.com", "-id", stored.ID.String(), "-yes")

	if got.Code != gonsole.ExitDone || got.Stdout != "revoked token "+stored.ID.String()+"\n" {
		t.Fatalf("token:revoke -yes = %d, stdout %q, stderr %q, want 0 and the token named",
			got.Code, got.Stdout, got.Stderr)
	}
	_, err := postgres.NewTokenStore(testPool(t, databaseURL)).ByHash(t.Context(), apitoken.HashSecret(secret))
	if !errors.Is(err, apitoken.ErrNotFound) {
		t.Errorf("ByHash() after token:revoke -yes error = %v, want the token gone", err)
	}
}

func TestTokenRevokeOnlyPreviewsUntilYes(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	secret := secretOf(t, mint(t, env).Stdout)
	stored := storedToken(t, databaseURL, secret)

	got := testkit.Run(t, bareProgram(env), "",
		"token:revoke", "-email", "admin@example.com", "-id", stored.ID.String())

	want := "would revoke token " + stored.ID.String() + " (n8n) of admin@example.com\n"
	if got.Code != gonsole.ExitDone || got.Stdout != want ||
		got.Stderr != "alphone: dry run, nothing changed, pass -yes to apply\n" {
		t.Errorf("token:revoke = %d, stdout %q, stderr %q, want 0, %q and a dry run",
			got.Code, got.Stdout, got.Stderr, want)
	}
	if kept := storedToken(t, databaseURL, secret); kept.ID != stored.ID {
		t.Errorf("the secret finds the stored token %v after the preview, want %v kept", kept.ID, stored.ID)
	}
}

func TestTokenRevokeRefusesATokenTheOwnerDoesNotHold(t *testing.T) {
	t.Parallel()

	modes := map[string][]string{"a preview": nil, "a revoke confirmed with -yes": {"-yes"}}
	for mode, confirm := range modes {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			databaseURL, env := tokenDatabase(t)
			createAccount(t, testGetenv(env), "maria.perez@example.com", "member")
			own := testkit.Run(t, bareProgram(env), "",
				"token:create", "-email", "maria.perez@example.com", "-name", "reporting")
			if own.Code != gonsole.ExitDone {
				t.Fatalf("token:create for maria.perez@example.com = %d with stderr %q, want 0", own.Code, own.Stderr)
			}
			secret := secretOf(t, mint(t, env).Stdout)
			held := storedToken(t, databaseURL, secret)
			args := []string{"token:revoke", "-email", "maria.perez@example.com", "-id", held.ID.String()}

			got := testkit.Run(t, bareProgram(env), "", append(args, confirm...)...)

			want := "alphone: " + apitoken.ErrNotFound.Error() + "\n"
			if got.Code != gonsole.ExitFailed || got.Stdout != "" || got.Stderr != want {
				t.Errorf("%s = %d, stdout %q, stderr %q, want 1 and %q", mode, got.Code, got.Stdout, got.Stderr, want)
			}
			if kept := storedToken(t, databaseURL, secret); kept.ID != held.ID {
				t.Errorf("the token %v is held after a revoke by another account, want %v kept", kept.ID, held.ID)
			}
		})
	}
}

func TestTokenRevokeRefusesAnUnreadableIDBeforeTheDatabase(t *testing.T) {
	t.Parallel()

	env := map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}
	want := "alphone: token:revoke: invalid value \"not-a-uuid\" for flag -id: invalid UUID length: 10\n\n" +
		helpPage(t, "token:revoke")
	modes := map[string][]string{"a preview": nil, "a revoke confirmed with -yes": {"-yes"}}
	for mode, confirm := range modes {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()

			args := append([]string{"token:revoke", "-email", "admin@example.com", "-id", "not-a-uuid"}, confirm...)
			got := testkit.Run(t, bareProgram(env), "", args...)

			if got.Code != gonsole.ExitMisused || got.Stdout != "" || got.Stderr != want {
				t.Errorf("%q = %d, stdout %q, stderr %q, want 2, no database reached and %q",
					args, got.Code, got.Stdout, got.Stderr, want)
			}
		})
	}
}

func TestATokenCommandUnderAnotherNameNamesItInAValueItCannotRead(t *testing.T) {
	t.Parallel()

	renamed := bareProgram(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})
	renamed.Commands = append(renamed.Commands,
		tokenCommand("token:mint", "mint a token", createTokenFlags, readCreate, "email", "name"),
		tokenCommand("token:drop", "drop a token", revokeTokenFlags, readRevoke, "email", "id"))
	lines := map[string]struct {
		args []string
		want string
	}{
		"a lifetime in no whole days": {
			[]string{"token:mint", "-email", "admin@example.com", "-name", "n8n", "-ttl", "soon"},
			`token:mint: invalid value "soon" for flag -ttl: want a whole number of days or never`,
		},
		"an id that is not a UUID": {
			[]string{"token:drop", "-email", "admin@example.com", "-id", "not-a-uuid"},
			`token:drop: invalid value "not-a-uuid" for flag -id: invalid UUID length: 10`,
		},
	}
	for condition, line := range lines {
		t.Run(condition, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, renamed, "", line.args...)

			want := "alphone: " + line.want + "\n"
			if got.Code != gonsole.ExitMisused || got.Stdout != "" || !strings.HasPrefix(got.Stderr, want) {
				t.Errorf("%q = %d, stdout %q, stderr %q, want 2, nothing on stdout and %q",
					line.args, got.Code, got.Stdout, got.Stderr, want)
			}
		})
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

// jsonMoment returns the moment as a JSON document writes it, in UTC.
func jsonMoment(at time.Time) string {
	return strconv.Quote(at.UTC().Format(time.RFC3339Nano))
}

// tokenDocument returns the document token:list -json answers for the one stored token, the fields before it given.
func tokenDocument(stored apitoken.Token, lastUsed, expires string, leading ...string) string {
	lines := []string{"{", `  "tokens": [`, "    {"}
	for _, field := range leading {
		lines = append(lines, "      "+field+",")
	}
	return strings.Join(append(lines,
		`      "id": "`+stored.ID.String()+`",`,
		`      "name": "`+stored.Name+`",`,
		`      "scopes": [`,
		`        "`+stored.Scopes.String()+`"`,
		"      ],",
		`      "created_at": `+jsonMoment(stored.CreatedAt)+",",
		`      "last_used_at": `+lastUsed+",",
		`      "expires_at": `+expires,
		"    }",
		"  ]",
		"}",
		"",
	), "\n")
}

func TestTokenListAnswersOneJSONDocument(t *testing.T) {
	t.Parallel()

	held := map[string]func(t *testing.T, databaseURL string, env map[string]string) string{
		"an account holding no token": func(*testing.T, string, map[string]string) string {
			return "{\n  \"tokens\": []\n}\n"
		},
		"a token never used that expires": func(t *testing.T, databaseURL string, env map[string]string) string {
			stored := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))
			return tokenDocument(stored, "null", jsonMoment(stored.ExpiresAt))
		},
		"a token used once that never expires": func(t *testing.T, databaseURL string, env map[string]string) string {
			stored := storedToken(t, databaseURL, secretOf(t, mint(t, env, "-ttl", "never").Stdout))
			used := time.Now().Truncate(time.Microsecond)
			if err := postgres.NewTokenStore(testPool(t, databaseURL)).TouchLastUsed(t.Context(), stored.ID, used); err != nil {
				t.Fatalf("TouchLastUsed() error = %v, want nil", err)
			}
			return tokenDocument(stored, jsonMoment(used), "null")
		},
	}
	for condition, holding := range held {
		t.Run(condition, func(t *testing.T) {
			t.Parallel()

			databaseURL, env := tokenDatabase(t)
			want := holding(t, databaseURL, env)

			got := testkit.Run(t, bareProgram(env), "", "token:list", "-email", "admin@example.com", "-json")

			if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
				t.Errorf("token:list -json = %d, stdout %q, stderr %q, want 0 and\n%s",
					got.Code, got.Stdout, got.Stderr, want)
			}
		})
	}
}

// allTokensLine returns the line token:list -all prints for the stored token of owner, never used, in the tenant.
func allTokensLine(owner string, tenantID uuid.UUID, stored apitoken.Token) string {
	return fmt.Sprintf("%s  tenant %s  %s  %s  scopes %s  created %s  last used never  expires %s",
		owner, tenantID, stored.ID, stored.Name, stored.Scopes, stored.CreatedAt.UTC().Format(dateLayout),
		stored.ExpiresAt.UTC().Format(dateLayout))
}

// everyTokenEntry is what a test reads back of one token the document token:list -all answers.
type everyTokenEntry struct {
	Owner    string `json:"owner"`
	TenantID string `json:"tenant_id"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}

// ownerlessToken stores a token in the default tenant whose user id names no account, answering it as stored.
func ownerlessToken(t *testing.T, databaseURL string, lifetime time.Duration) apitoken.Token {
	t.Helper()
	minted, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "orphan", apitoken.Full(), lifetime)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	store := postgres.NewTokenStore(testPool(t, databaseURL))
	if err := store.Create(sdk.WithTenant(t.Context(), tenant.DefaultID), minted.Token); err != nil {
		t.Fatalf("Create() error = %v, want the token stored in the default tenant", err)
	}
	return storedToken(t, databaseURL, minted.Secret)
}

func TestTokenListAllCoversEveryAccount(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	_, acme := placedMember(t, databaseURL, env)
	automation := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))
	reporting := storedToken(t, databaseURL, secretOf(t, testkit.Run(t, bareProgram(env), "",
		"token:create", "-email", "maria.perez@example.com", "-name", "reporting").Stdout))
	orphan := ownerlessToken(t, databaseURL, 90*24*time.Hour)

	text := testkit.Run(t, bareProgram(env), "", "token:list", "-all")
	document := testkit.Run(t, bareProgram(env), "", "token:list", "-all", "-json")

	lines := strings.Split(strings.TrimSuffix(text.Stdout, "\n"), "\n")
	slices.Sort(lines)
	want := []string{
		allTokensLine("admin@example.com", tenant.DefaultID, automation),
		allTokensLine("maria.perez@example.com", acme, reporting),
		allTokensLine("(no account)", tenant.DefaultID, orphan),
	}
	slices.Sort(want)
	if text.Code != gonsole.ExitDone || !slices.Equal(lines, want) {
		t.Errorf("token:list -all = %d, stdout %q, stderr %q, want 0 and the lines %q",
			text.Code, text.Stdout, text.Stderr, want)
	}
	var listed struct {
		Tokens []everyTokenEntry `json:"tokens"`
	}
	err := json.Unmarshal([]byte(document.Stdout), &listed)
	entries := []everyTokenEntry{
		{"admin@example.com", tenant.DefaultID.String(), automation.ID.String(), "n8n"},
		{"maria.perez@example.com", acme.String(), reporting.ID.String(), "reporting"},
		{"", tenant.DefaultID.String(), orphan.ID.String(), "orphan"},
	}
	byID := func(a, b everyTokenEntry) int { return strings.Compare(a.ID, b.ID) }
	slices.SortFunc(listed.Tokens, byID)
	slices.SortFunc(entries, byID)
	if err != nil || document.Code != gonsole.ExitDone || !slices.Equal(listed.Tokens, entries) {
		t.Errorf("token:list -all -json = %d, stdout %q, stderr %q (%v), want 0 and the tokens %+v",
			document.Code, document.Stdout, document.Stderr, err, entries)
	}
}

func TestTokenListRefusesAllTogetherWithAnAddress(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "token:list", "-all", "-email", "admin@example.com")

	want := "alphone: token:list takes -email or -all, not both\n"
	if got.Code != gonsole.ExitMisused || got.Stdout != "" || !strings.HasPrefix(got.Stderr, want) {
		t.Errorf("token:list -all -email = %d, stdout %q, stderr %q, want 2 and %q",
			got.Code, got.Stdout, got.Stderr, want)
	}
}

func TestTokenListAllAnswersOneJSONDocument(t *testing.T) {
	t.Parallel()

	inDefault := `"tenant_id": "` + tenant.DefaultID.String() + `"`
	held := map[string]func(t *testing.T, databaseURL string, env map[string]string) string{
		"an install holding no token": func(*testing.T, string, map[string]string) string {
			return "{\n  \"tokens\": []\n}\n"
		},
		"a token of an account never used": func(t *testing.T, databaseURL string, env map[string]string) string {
			stored := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))
			return tokenDocument(stored, "null", jsonMoment(stored.ExpiresAt), `"owner": "admin@example.com"`, inDefault)
		},
		"a token used once whose account is gone": func(t *testing.T, databaseURL string, _ map[string]string) string {
			stored := ownerlessToken(t, databaseURL, apitoken.Never)
			used := time.Now().Truncate(time.Microsecond)
			if err := postgres.NewTokenStore(testPool(t, databaseURL)).TouchLastUsed(t.Context(), stored.ID, used); err != nil {
				t.Fatalf("TouchLastUsed() error = %v, want nil", err)
			}
			return tokenDocument(stored, jsonMoment(used), "null", `"owner": null`, inDefault)
		},
	}
	for condition, holding := range held {
		t.Run(condition, func(t *testing.T) {
			t.Parallel()

			databaseURL, env := tokenDatabase(t)
			want := holding(t, databaseURL, env)

			got := testkit.Run(t, bareProgram(env), "", "token:list", "-all", "-json")

			if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
				t.Errorf("token:list -all -json = %d, stdout %q, stderr %q, want 0 and\n%s",
					got.Code, got.Stdout, got.Stderr, want)
			}
		})
	}
}

func TestTokenListWithAllSetToFalseListsTheNamedAccount(t *testing.T) {
	t.Parallel()

	databaseURL, env := tokenDatabase(t)
	stored := storedToken(t, databaseURL, secretOf(t, mint(t, env).Stdout))

	got := testkit.Run(t, bareProgram(env), "", "token:list", "-all=false", "-email", "admin@example.com")

	want := stored.ID.String() + "  n8n  scopes "
	if got.Code != gonsole.ExitDone || !strings.HasPrefix(got.Stdout, want) {
		t.Errorf("token:list -all=false -email = %d, stdout %q, stderr %q, want 0 and the account's line %q",
			got.Code, got.Stdout, got.Stderr, want)
	}
}

func TestTokenListHelpNamesItsFlags(t *testing.T) {
	t.Parallel()

	got := testkit.Run(t, bareProgram(nil), "", "token:list", "-h")

	want := strings.Join([]string{
		"list the tokens of one account or of every account",
		"",
		"Usage:",
		"  alphone token:list [flags]",
		"",
		"Flags:",
		"  -all",
		"    \tlist the tokens of every account in every tenant, each with its owner and its tenant",
		"  -email address",
		"    \taddress of the account that owns the tokens",
		"  -json",
		"    \tanswer one JSON document",
		"",
	}, "\n")
	if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
		t.Errorf("token:list -h = %d, stdout %q, stderr %q, want 0 and its page\n%s",
			got.Code, got.Stdout, got.Stderr, want)
	}
}

func TestTokenCommandsWantEveryFlagTheyNeed(t *testing.T) {
	t.Parallel()

	const (
		createOwner = "token:create wants -email <address>"
		createName  = "token:create wants -name <name>"
		listOwner   = "token:list wants -email <address> or -all"
		revokeOwner = "token:revoke wants -email <address>"
		revokeID    = "token:revoke wants -id <id>"
		owner       = "admin@example.com"
	)
	unheldID := uuid.Nil.String()
	lines := map[string]struct {
		args []string
		want string
	}{
		"token:create with a blank -email":   {[]string{"token:create", "-email", "  ", "-name", "n8n"}, createOwner},
		"token:create with no -email":        {[]string{"token:create", "-name", "n8n"}, createOwner},
		"token:create with a blank -name":    {[]string{"token:create", "-email", owner, "-name", "  "}, createName},
		"token:create with no -name":         {[]string{"token:create", "-email", owner}, createName},
		"token:list with a blank -email":     {[]string{"token:list", "-email", "  "}, listOwner},
		"token:list with no -email":          {[]string{"token:list"}, listOwner},
		"token:list with -all=false alone":   {[]string{"token:list", "-all=false"}, listOwner},
		"token:revoke with a blank -email":   {[]string{"token:revoke", "-email", "  ", "-id", unheldID}, revokeOwner},
		"token:revoke with no -email":        {[]string{"token:revoke", "-id", unheldID}, revokeOwner},
		"token:revoke with a blank -id":      {[]string{"token:revoke", "-email", owner, "-id", "  "}, revokeID},
		"token:revoke with no -id":           {[]string{"token:revoke", "-email", owner}, revokeID},
		"token:revoke -yes with a blank -id": {[]string{"token:revoke", "-email", owner, "-id", "  ", "-yes"}, revokeID},
		"token:revoke -yes with no -id":      {[]string{"token:revoke", "-email", owner, "-yes"}, revokeID},
	}
	for testName, line := range lines {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			got := testkit.Run(t, bareProgram(nil), "", line.args...)

			want := "alphone: " + line.want + "\n"
			if got.Code != gonsole.ExitMisused || got.Stdout != "" || !strings.HasPrefix(got.Stderr, want) {
				t.Errorf("%q = %d, stdout %q, stderr %q, want 2, nothing on stdout and %q",
					line.args, got.Code, got.Stdout, got.Stderr, want)
			}
		})
	}
}

func TestMustMintPanicsOnAnError(t *testing.T) {
	t.Parallel()

	failed := errors.New("no token")
	defer func() {
		if recovered := recover(); recovered != failed {
			t.Fatalf("mustMint() recovered %v, want a panic with the error", recovered)
		}
	}()

	mustMint(apitoken.Minted{}, failed)
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
	owner := gouncer.User{ID: uuid.Must(uuid.NewV7()), Email: "admin@example.com"}
	call := gonsole.Call{
		Flags: map[string]string{"name": "n8n", "id": uuid.Nil.String(), "ttl": neverWord}, Stdout: io.Discard,
	}
	reads := map[string]tokenRead{"token:create": readCreate, "token:list": readList, "token:revoke": readRevoke}

	for name, read := range reads {
		step, err := read(name, call)
		if err != nil {
			t.Fatalf("reading the %s line error = %v, want its step", name, err)
		}
		if err := step(t.Context(), store, owner, call); err == nil {
			t.Errorf("the %s step on a closed pool error = nil, want the failure reported", name)
		}
	}
	if err := listEveryToken(t.Context(), store, call); err == nil {
		t.Error("the token:list -all step on a closed pool error = nil, want the failure reported")
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
		"    \tdays the token lasts, or never, ALPHONE_TOKEN_TTL_DAYS days when left out, 90 when it is unset",
		"",
	}, "\n")
	if got.Code != gonsole.ExitDone || got.Stdout != want || got.Stderr != "" {
		t.Errorf("token:create -h = %d, stdout %q, stderr %q, want 0 and its page\n%s",
			got.Code, got.Stdout, got.Stderr, want)
	}
}
