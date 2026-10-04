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

// jsonMoment returns the moment as a JSON document writes it, in UTC.
func jsonMoment(at time.Time) string {
	return strconv.Quote(at.UTC().Format(time.RFC3339Nano))
}

// tokenDocument returns the document token:list -json answers for the one stored token, its last use and expiry given.
func tokenDocument(stored apitoken.Token, lastUsed, expires string) string {
	return strings.Join([]string{
		"{",
		`  "tokens": [`,
		"    {",
		`      "id": "` + stored.ID.String() + `",`,
		`      "name": "` + stored.Name + `",`,
		`      "scopes": [`,
		`        "` + stored.Scopes.String() + `"`,
		"      ],",
		`      "created_at": ` + jsonMoment(stored.CreatedAt) + ",",
		`      "last_used_at": ` + lastUsed + ",",
		`      "expires_at": ` + expires,
		"    }",
		"  ]",
		"}",
		"",
	}, "\n")
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
	owner := gouncer.User{ID: uuid.Must(uuid.NewV7()), Email: "admin@example.com"}
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
