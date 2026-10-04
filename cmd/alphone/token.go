// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"flag"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
)

// defaultTokenLifetime is how long a token minted from the command line lasts.
const defaultTokenLifetime = 90 * 24 * time.Hour

// dateLayout formats the dates the token commands print.
const dateLayout = "2006-01-02"

// neverWord is the lifetime a token is given to be permanent.
const neverWord = "never"

// tokenStep is the work one token command does over the token store for the account that owns the tokens.
type tokenStep func(ctx context.Context, tokens *postgres.TokenStore, owner uuid.UUID, call gonsole.Call) error

// tokenCommands returns the commands that mint, list and revoke the API tokens of one account.
func tokenCommands() []gonsole.Command {
	return []gonsole.Command{
		tokenCommand("token:create", "mint a token for one account and show its secret once", createTokenFlags,
			createToken),
		tokenCommand("token:list", "list the tokens of one account", ownerFlag, listTokens),
		tokenCommand("token:revoke", "revoke one token of one account", revokeTokenFlags, revokeToken),
	}
}

// tokenCommand returns the token command called name, which runs step for the account its -email flag names.
func tokenCommand(name, summary string, flags func(*flag.FlagSet), step tokenStep) gonsole.Command {
	return gonsole.Command{
		Name:    name,
		Summary: summary,
		Flags:   flags,
		Run: func(ctx context.Context, call gonsole.Call) error {
			email := call.Flags["email"]
			if strings.TrimSpace(email) == "" {
				return gonsole.Misuse(fmt.Errorf("%s wants -email <address>", name))
			}
			address, err := call.DatabaseURL()
			if err != nil {
				return err
			}
			pool, err := pgxpool.New(ctx, address)
			if err != nil {
				return fmt.Errorf("parse database url: %w", err)
			}
			defer pool.Close()
			owner, err := authkitpg.NewUserStore(pool).UserByEmail(ctx, email)
			if err != nil {
				return err
			}
			return step(ctx, postgres.NewTokenStore(pool), owner.ID, call)
		},
	}
}

// ownerFlag declares the -email flag naming the account that owns the tokens.
func ownerFlag(fs *flag.FlagSet) {
	fs.String("email", "", "`address` of the account that owns the tokens")
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
func createToken(ctx context.Context, tokens *postgres.TokenStore, owner uuid.UUID, call gonsole.Call) error {
	lifetime, err := tokenLifetime(call)
	if err != nil {
		return err
	}
	granted := grantedScopes(call)
	if err := graphres.ValidateScopes(granted); err != nil {
		return err
	}
	minted, err := apitoken.Mint(owner, call.Flags["name"], granted, lifetime)
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

// listTokens prints one line per token of the owner, secrets excluded.
func listTokens(ctx context.Context, tokens *postgres.TokenStore, owner uuid.UUID, call gonsole.Call) error {
	stored, err := tokens.ListForUser(ctx, owner)
	if err != nil {
		return err
	}
	for _, t := range stored {
		_, _ = fmt.Fprintf(call.Stdout, "%s  %s  scopes %s  created %s  last used %s  expires %s\n",
			t.ID, t.Name, t.Scopes, t.CreatedAt.UTC().Format(dateLayout),
			orNever(t.LastUsedAt), orNever(t.ExpiresAt))
	}
	return nil
}

// orNever returns the date in UTC, or never when the moment has not come.
func orNever(at time.Time) string {
	if at.IsZero() {
		return "never"
	}
	return at.UTC().Format(dateLayout)
}

// revokeToken deletes the token of the owner the call's -id flag names.
func revokeToken(ctx context.Context, tokens *postgres.TokenStore, owner uuid.UUID, call gonsole.Call) error {
	tokenID, err := uuid.Parse(call.Flags["id"])
	if err != nil {
		return fmt.Errorf("parse token id: %w", err)
	}
	if err := tokens.Revoke(ctx, owner, tokenID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(call.Stdout, "revoked token %s\n", tokenID)
	return nil
}
