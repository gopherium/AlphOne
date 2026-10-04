// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// defaultTokenLifetime is how long a token minted from the command line lasts.
const defaultTokenLifetime = 90 * 24 * time.Hour

// dateLayout formats the dates the token commands print.
const dateLayout = "2006-01-02"

// neverWord is the lifetime a token is given to be permanent.
const neverWord = "never"

// noAccount stands for the owner in the token list when no account answers for a token.
const noAccount = "(no account)"

// tokenStep is the work one token command does over the token store for the account that owns the tokens.
type tokenStep func(ctx context.Context, tokens *postgres.TokenStore, owner gouncer.User, call gonsole.Call) error

// tokenCommands returns the commands that mint, list and revoke the API tokens of an account.
func tokenCommands() []gonsole.Command {
	revoke := tokenCommand("token:revoke", "revoke one token of one account", revokeTokenFlags, revokeToken)
	revoke.Writes = true
	return []gonsole.Command{
		tokenCommand("token:create", "mint a token for one account and show its secret once", createTokenFlags,
			createToken),
		tokenListCommand(),
		revoke,
	}
}

// tokenCommand returns the token command called name, running step in the tenant of the account -email names.
func tokenCommand(name, summary string, flags func(*flag.FlagSet), step tokenStep) gonsole.Command {
	return gonsole.Command{
		Name:    name,
		Summary: summary,
		Flags:   flags,
		Run: func(ctx context.Context, call gonsole.Call) error {
			email := storedAddress(call.Flags["email"])
			if email == "" {
				return gonsole.Misuse(fmt.Errorf("%s wants -email <address>", name))
			}
			return withPool(ctx, call, func(pool *pgxpool.Pool) error {
				standing, owner, err := tokenOwner(ctx, pool, email)
				if err != nil {
					return err
				}
				return step(standing, postgres.NewTokenStore(pool), owner, call)
			})
		},
	}
}

// tokenListCommand returns token:list, which lists the tokens of the account -email names or of every account.
func tokenListCommand() gonsole.Command {
	list := tokenCommand("token:list", "list the tokens of one account or of every account", listTokenFlags, listTokens)
	list.JSON = true
	ofOne := list.Run
	list.Run = func(ctx context.Context, call gonsole.Call) error {
		_, named := call.Flags["email"]
		every := call.Flags["all"] == "true"
		switch {
		case every && named:
			return gonsole.Misuse(errors.New("token:list takes -email or -all, not both"))
		case every:
			return withPool(ctx, call, func(pool *pgxpool.Pool) error {
				return listEveryToken(ctx, postgres.NewTokenStore(pool), call)
			})
		case storedAddress(call.Flags["email"]) == "":
			return gonsole.Misuse(errors.New("token:list wants -email <address> or -all"))
		}
		return ofOne(ctx, call)
	}
	return list
}

// withPool runs work over a pool on the database the call's settings name, closing the pool once work returns.
func withPool(ctx context.Context, call gonsole.Call, work func(*pgxpool.Pool) error) error {
	address, err := call.DatabaseURL()
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, address)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	return work(pool)
}

// storedAddress returns typed as gouncer stores an address, trimmed and in lower case.
func storedAddress(typed string) string {
	return strings.ToLower(strings.TrimSpace(typed))
}

// tokenOwner returns the account at email and ctx standing in that account's tenant.
func tokenOwner(ctx context.Context, pool *pgxpool.Pool, email string) (context.Context, gouncer.User, error) {
	owner, err := authkitpg.NewUserStore(pool).UserByEmail(ctx, email)
	if err != nil {
		return ctx, gouncer.User{}, err
	}
	held, err := postgres.NewTenantStore(pool).TenantForUser(ctx, owner.ID)
	if err != nil {
		return ctx, gouncer.User{}, err
	}
	return sdk.WithTenant(ctx, held.ID), owner, nil
}

// ownerFlag declares the -email flag naming the account that owns the tokens.
func ownerFlag(fs *flag.FlagSet) {
	fs.String("email", "", "`address` of the account that owns the tokens")
}

// listTokenFlags declares the flags of token:list.
func listTokenFlags(fs *flag.FlagSet) {
	ownerFlag(fs)
	fs.Bool("all", false, "list the tokens of every account in every tenant, each with its owner and its tenant")
}

// createTokenFlags declares the flags of token:create.
func createTokenFlags(fs *flag.FlagSet) {
	ownerFlag(fs)
	fs.String("name", "", "`name` of the token to create")
	fs.Var(new(scopeList), "scope", "`area:access` scope the token may act in, repeatable, the area one of "+
		strings.Join(graphres.DeclaredAreas(), ", "))
	fs.String("ttl", "", "`days` the token lasts, or never")
}

// revokeTokenFlags declares the flags of token:revoke.
func revokeTokenFlags(fs *flag.FlagSet) {
	ownerFlag(fs)
	fs.String("id", "", "`id` of the token to revoke")
}

// scopeList collects a repeatable scope flag.
type scopeList []string

// String returns the scopes collected so far, space separated.
func (l *scopeList) String() string {
	return strings.Join(*l, " ")
}

// Set adds one scope to the collection, refusing a blank scope or one that holds a space.
func (l *scopeList) Set(scope string) error {
	if scope == "" || strings.ContainsFunc(scope, unicode.IsSpace) {
		return fmt.Errorf("%w: %q", apitoken.ErrMalformedScope, scope)
	}
	*l = append(*l, scope)
	return nil
}

// grantedScopes returns the scopes the call asks for, the wildcard when it asks for none.
func grantedScopes(call gonsole.Call) apitoken.Scopes {
	asked := strings.Fields(call.Flags["scope"])
	if len(asked) == 0 {
		return apitoken.Full()
	}
	return apitoken.Scopes(asked)
}

// tokenLifetime returns the lifetime the call asks for, the default when it asks for none.
func tokenLifetime(call gonsole.Call) (time.Duration, error) {
	ttl := call.Flags["ttl"]
	switch ttl {
	case "":
		return defaultTokenLifetime, nil
	case neverWord:
		return apitoken.Never, nil
	}
	days, err := strconv.Atoi(ttl)
	if err != nil {
		return 0, fmt.Errorf("parse ttl: %w", err)
	}
	return apitoken.LifetimeOfDays(days)
}

// createToken mints the token the call asks for and prints its secret for the only time.
func createToken(ctx context.Context, tokens *postgres.TokenStore, owner gouncer.User, call gonsole.Call) error {
	lifetime, err := tokenLifetime(call)
	if err != nil {
		return err
	}
	granted := grantedScopes(call)
	if err := graphres.ValidateScopes(granted); err != nil {
		return err
	}
	minted, err := apitoken.Mint(owner.ID, call.Flags["name"], granted, lifetime)
	if err != nil {
		return err
	}
	if err := tokens.Create(ctx, minted.Token); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(call.Stdout, "created token %s\n", minted.Token.ID)
	_, _ = fmt.Fprintf(call.Stdout, "secret: %s\n", minted.Secret)
	_, _ = fmt.Fprintln(call.Stdout, "store it now, it is never shown again")
	_, _ = fmt.Fprintf(call.Stdout, "scopes %s, expires %s\n", minted.Token.Scopes, orNever(minted.Token.ExpiresAt))
	return nil
}

// listTokens prints one line per token of the owner, or one JSON document with -json, secrets excluded.
func listTokens(ctx context.Context, tokens *postgres.TokenStore, owner gouncer.User, call gonsole.Call) error {
	stored, err := tokens.ListForUser(ctx, owner.ID)
	if err != nil {
		return err
	}
	if call.JSON {
		listed := make([]listedToken, 0, len(stored))
		for _, t := range stored {
			listed = append(listed, listedTokenOf(t))
		}
		return call.Encode(struct {
			Tokens []listedToken `json:"tokens"`
		}{listed})
	}
	for _, t := range stored {
		_, _ = io.WriteString(call.Stdout, tokenLine(t))
	}
	return nil
}

// listEveryToken prints one line per token of every account, or one JSON document with -json, owner and tenant first.
func listEveryToken(ctx context.Context, tokens *postgres.TokenStore, call gonsole.Call) error {
	every, err := tokens.ListEvery(ctx)
	if err != nil {
		return err
	}
	if call.JSON {
		listed := make([]accountToken, 0, len(every))
		for _, held := range every {
			listed = append(listed, accountToken{
				Owner: addressOrNull(held.Owner), TenantID: held.TenantID.String(), listedToken: listedTokenOf(held.Token),
			})
		}
		return call.Encode(struct {
			Tokens []accountToken `json:"tokens"`
		}{listed})
	}
	for _, held := range every {
		owner := cmp.Or(held.Owner, noAccount)
		_, _ = fmt.Fprintf(call.Stdout, "%s  tenant %s  %s", owner, held.TenantID, tokenLine(held.Token))
	}
	return nil
}

// addressOrNull returns the address of a token's owner, nil when no account answers for the token.
func addressOrNull(address string) *string {
	if address == "" {
		return nil
	}
	return &address
}

// tokenLine returns the line that lists one token, its secret left out.
func tokenLine(t apitoken.Token) string {
	return fmt.Sprintf("%s  %s  scopes %s  created %s  last used %s  expires %s\n",
		t.ID, t.Name, t.Scopes, t.CreatedAt.UTC().Format(dateLayout), orNever(t.LastUsedAt), orNever(t.ExpiresAt))
}

// accountToken is one token in the document token:list -all answers, after its owner's address and its tenant.
type accountToken struct {
	Owner    *string `json:"owner"`
	TenantID string  `json:"tenant_id"`
	listedToken
}

// listedToken is one token in the document token:list answers with -json, its secret left out.
type listedToken struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// listedTokenOf returns the stored token as the document token:list answers lists it.
func listedTokenOf(t apitoken.Token) listedToken {
	return listedToken{
		ID:         t.ID.String(),
		Name:       t.Name,
		Scopes:     append([]string{}, t.Scopes...),
		CreatedAt:  t.CreatedAt.UTC(),
		LastUsedAt: momentOrNull(t.LastUsedAt),
		ExpiresAt:  momentOrNull(t.ExpiresAt),
	}
}

// momentOrNull returns the moment in UTC, nil when it has not come.
func momentOrNull(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	utc := at.UTC()
	return &utc
}

// orNever returns the date in UTC, or never when the moment has not come.
func orNever(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return at.UTC().Format(dateLayout)
}

// revokeToken deletes the token of the owner the call's -id flag names, only naming it until the call applies.
func revokeToken(ctx context.Context, tokens *postgres.TokenStore, owner gouncer.User, call gonsole.Call) error {
	tokenID, err := uuid.Parse(call.Flags["id"])
	if err != nil {
		return fmt.Errorf("parse token id: %w", err)
	}
	if !call.Apply {
		return previewRevoke(ctx, tokens, owner, tokenID, call)
	}
	if err := tokens.Revoke(ctx, owner.ID, tokenID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(call.Stdout, "revoked token %s\n", tokenID)
	return nil
}

// previewRevoke names the token of the owner a revoke would delete, apitoken.ErrNotFound when the owner holds none.
func previewRevoke(
	ctx context.Context, tokens *postgres.TokenStore, owner gouncer.User, tokenID uuid.UUID, call gonsole.Call,
) error {
	held, err := tokens.ListForUser(ctx, owner.ID)
	if err != nil {
		return err
	}
	for _, t := range held {
		if t.ID == tokenID {
			_, _ = fmt.Fprintf(call.Stdout, "would revoke token %s (%s) of %s\n", t.ID, t.Name, owner.Email)
			return nil
		}
	}
	return apitoken.ErrNotFound
}
