// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	gqlclient "github.com/99designs/gqlgen/client"
	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
)

// mintedToken is the answer the mint mutation carries.
type mintedToken struct {
	APITokenCreate struct {
		Secret string `json:"secret"`
		Token  struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Scopes    []string `json:"scopes"`
			ExpiresAt *string  `json:"expiresAt"`
		} `json:"token"`
	} `json:"apiTokenCreate"`
}

// listedTokens is the answer the token listing carries.
type listedTokens struct {
	APITokens []struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		Scopes    []string `json:"scopes"`
		ExpiresAt *string  `json:"expiresAt"`
	} `json:"apiTokens"`
}

// newTokenResolver returns a resolver over a real token store, acting as one owner.
func newTokenResolver(t *testing.T) (*gqlclient.Client, *postgres.TokenStore, uuid.UUID) {
	t.Helper()
	pool := newTestPool(t)
	tokens := postgres.NewTokenStore(pool)
	owner := uuid.Must(uuid.NewV7())
	resolver := &graphres.Resolver{Version: "9.9.9", Tokens: tokens}
	return newGraphClient(t, resolver, owner), tokens, owner
}

func TestAPITokenCreateAnswersTheSecretOnce(t *testing.T) {
	t.Parallel()

	client, tokens, owner := newTokenResolver(t)

	var minted mintedToken
	client.MustPost(`mutation { apiTokenCreate(name: "automation", scopes: ["tasks:write"], ttlDays: 30)
		{ secret token { id name scopes expiresAt } } }`, &minted)

	if !strings.HasPrefix(minted.APITokenCreate.Secret, apitoken.Prefix) {
		t.Errorf("secret = %q, want the %q prefix", minted.APITokenCreate.Secret, apitoken.Prefix)
	}
	if got := minted.APITokenCreate.Token.Name; got != "automation" {
		t.Errorf("name = %q, want automation", got)
	}
	if got := minted.APITokenCreate.Token.Scopes; len(got) != 1 || got[0] != "tasks:write" {
		t.Errorf("scopes = %v, want [tasks:write]", got)
	}
	if minted.APITokenCreate.Token.ExpiresAt == nil {
		t.Error("expiresAt is null, want the thirty day expiry")
	}
	stored, err := tokens.ListForUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil", err)
	}
	if len(stored) != 1 {
		t.Fatalf("stored %d tokens, want 1", len(stored))
	}
	if stored[0].Hash != apitoken.HashSecret(minted.APITokenCreate.Secret) {
		t.Error("the stored hash does not match the answered secret")
	}
}

func TestAPITokenCreateLeavesTheExpiryOpenWithoutATTL(t *testing.T) {
	t.Parallel()

	client, _, _ := newTokenResolver(t)

	var minted mintedToken
	client.MustPost(`mutation { apiTokenCreate(name: "forever", scopes: ["*"])
		{ secret token { expiresAt } } }`, &minted)

	if minted.APITokenCreate.Token.ExpiresAt != nil {
		t.Errorf("expiresAt = %v, want null without a ttl", *minted.APITokenCreate.Token.ExpiresAt)
	}
}

func TestAPITokenCreateRefusesAMalformedScope(t *testing.T) {
	t.Parallel()

	client, _, _ := newTokenResolver(t)

	answered, err := client.RawPost(`mutation { apiTokenCreate(name: "bad", scopes: ["tasks:admin"])
		{ secret } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, answered.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
}

func TestAPITokenCreateRefusesALifetimeThatWouldOverflow(t *testing.T) {
	t.Parallel()

	client, tokens, owner := newTokenResolver(t)

	answered, err := client.RawPost(
		`mutation { apiTokenCreate(name: "huge", scopes: ["tasks:read"], ttlDays: 213504) { secret } }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, answered.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
	stored, err := tokens.ListForUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil", err)
	}
	if len(stored) != 0 {
		t.Errorf("stored %d tokens, want none, an overflowing lifetime mints nothing", len(stored))
	}
}

func TestAPITokenCreateAcceptsTheLongestLifetime(t *testing.T) {
	t.Parallel()

	client, _, _ := newTokenResolver(t)

	var minted mintedToken
	client.MustPost(fmt.Sprintf(
		`mutation { apiTokenCreate(name: "long", scopes: ["tasks:read"], ttlDays: %d)
			{ secret token { expiresAt } } }`, apitoken.MaxLifetimeDays), &minted)

	if minted.APITokenCreate.Token.ExpiresAt == nil {
		t.Error("expiresAt is null, want the longest lifetime honoured")
	}
}

func TestAPITokensListsTheCallersOwnTokensWithoutSecrets(t *testing.T) {
	t.Parallel()

	client, tokens, _ := newTokenResolver(t)
	var mine mintedToken
	client.MustPost(`mutation { apiTokenCreate(name: "mine", scopes: ["tasks:read"]) { secret } }`, &mine)
	theirs, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "theirs", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	if err := tokens.Create(t.Context(), theirs.Token); err != nil {
		t.Fatalf("storing another owner's token: %v", err)
	}

	var listed listedTokens
	client.MustPost(`{ apiTokens { id name scopes expiresAt } }`, &listed)

	if len(listed.APITokens) != 1 {
		t.Fatalf("listed %d tokens, want only the caller's own", len(listed.APITokens))
	}
	if got := listed.APITokens[0].Name; got != "mine" {
		t.Errorf("name = %q, want mine", got)
	}
}

func TestAPITokenCarriesNoSecretField(t *testing.T) {
	t.Parallel()

	client, _, _ := newTokenResolver(t)

	answered, err := client.RawPost(`{ apiTokens { id secret } }`)

	if err == nil && len(answered.Errors) == 0 {
		t.Error("a listed token answered a secret, want the schema to carry no such field")
	}
}

func TestAPITokenRevokeStopsOneOfTheCallersTokens(t *testing.T) {
	t.Parallel()

	client, tokens, owner := newTokenResolver(t)
	var minted mintedToken
	client.MustPost(`mutation { apiTokenCreate(name: "doomed", scopes: ["tasks:read"])
		{ token { id } } }`, &minted)

	var revoked struct {
		APITokenRevoke bool `json:"apiTokenRevoke"`
	}
	client.MustPost(`mutation { apiTokenRevoke(id: "`+minted.APITokenCreate.Token.ID+`") }`, &revoked)

	if !revoked.APITokenRevoke {
		t.Error("apiTokenRevoke() = false, want true")
	}
	stored, err := tokens.ListForUser(t.Context(), owner)
	if err != nil {
		t.Fatalf("ListForUser() error = %v, want nil", err)
	}
	if len(stored) != 0 {
		t.Errorf("stored %d tokens, want the revoked one gone", len(stored))
	}
}

func TestAPITokensShowsWhenATokenWasLastUsed(t *testing.T) {
	t.Parallel()

	client, tokens, owner := newTokenResolver(t)
	client.MustPost(`mutation { apiTokenCreate(name: "used", scopes: ["tasks:read"]) { secret } }`, &mintedToken{})
	stored, err := tokens.ListForUser(t.Context(), owner)
	if err != nil || len(stored) != 1 {
		t.Fatalf("ListForUser() = %v, %v, want one token", stored, err)
	}
	if err := tokens.TouchLastUsed(t.Context(), stored[0].ID, time.Now().UTC()); err != nil {
		t.Fatalf("TouchLastUsed() error = %v, want nil", err)
	}

	var listed struct {
		APITokens []struct {
			LastUsedAt *string `json:"lastUsedAt"`
		} `json:"apiTokens"`
	}
	client.MustPost(`{ apiTokens { lastUsedAt } }`, &listed)

	if len(listed.APITokens) != 1 || listed.APITokens[0].LastUsedAt == nil {
		t.Errorf("lastUsedAt = %v, want the moment the token acted", listed.APITokens)
	}
}

func TestAPITokenResolversReportAStoreFailure(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	owner := uuid.Must(uuid.NewV7())
	resolver := &graphres.Resolver{Version: "9.9.9", Tokens: postgres.NewTokenStore(pool)}
	client := newGraphClient(t, resolver, owner)
	pool.Close()

	listed, err := client.RawPost(`{ apiTokens { id } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(listed.Errors) == 0 {
		t.Error("apiTokens answered no error on a closed pool, want one")
	}

	minted, err := client.RawPost(`mutation { apiTokenCreate(name: "n8n", scopes: ["tasks:read"]) { secret } }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(minted.Errors) == 0 {
		t.Error("apiTokenCreate answered no error on a closed pool, want one")
	}
}

func TestAPITokenRevokeRefusesSomeoneElsesToken(t *testing.T) {
	t.Parallel()

	client, tokens, _ := newTokenResolver(t)
	theirs, err := apitoken.Mint(uuid.Must(uuid.NewV7()), "theirs", apitoken.Full(), apitoken.Never)
	if err != nil {
		t.Fatalf("apitoken.Mint() error = %v, want nil", err)
	}
	if err := tokens.Create(t.Context(), theirs.Token); err != nil {
		t.Fatalf("storing another owner's token: %v", err)
	}

	answered, err := client.RawPost(`mutation { apiTokenRevoke(id: "` + theirs.Token.ID.String() + `") }`)

	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if len(answered.Errors) == 0 {
		t.Error("errors = none, want a refusal for a token the caller does not own")
	}
}
