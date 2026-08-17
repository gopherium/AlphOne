// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
)

// defaultTokenLifetime is how long a token minted from the command line lasts.
const defaultTokenLifetime = 90 * 24 * time.Hour

// dateLayout formats the dates the token subcommands print.
const dateLayout = "2006-01-02"

// token creates, lists, and revokes the API tokens of one user.
func token(ctx context.Context, getenv func(string) string, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("token: want one of create, list, revoke")
	}
	verb := args[0]
	opts, err := parseTokenFlags(verb, args[1:], stdout)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}

	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := migrateSchemas(ctx, databaseURL); err != nil {
		return err
	}

	owner, err := authkitpg.NewUserStore(pool).UserByEmail(ctx, opts.email)
	if err != nil {
		return err
	}
	return runTokenVerb(ctx, postgres.NewTokenStore(pool), owner.ID, verb, opts, stdout)
}

// neverWord is the lifetime a token is given to be permanent.
const neverWord = "never"

// scopeList collects a repeatable scope flag.
type scopeList []string

// String returns the scopes collected so far, space separated.
func (l *scopeList) String() string {
	return strings.Join(*l, " ")
}

// Set adds one scope to the collection.
func (l *scopeList) Set(scope string) error {
	*l = append(*l, scope)
	return nil
}

// tokenFlags carries the parsed flags of a token subcommand.
type tokenFlags struct {
	email  string
	name   string
	id     string
	scopes scopeList
	ttl    string
}

// parseTokenFlags parses the flags of one token subcommand.
func parseTokenFlags(verb string, args []string, stdout io.Writer) (tokenFlags, error) {
	flags := flag.NewFlagSet("token "+verb, flag.ContinueOnError)
	flags.SetOutput(stdout)
	opts := tokenFlags{}
	flags.StringVar(&opts.email, "email", "", "email address of the owning user")
	flags.StringVar(&opts.name, "name", "", "name of the token to create")
	flags.StringVar(&opts.id, "id", "", "identifier of the token to revoke")
	flags.Var(&opts.scopes, "scope", "area and access the token may act in, repeatable")
	flags.StringVar(&opts.ttl, "ttl", "", "days the token lasts, or never")
	if err := flags.Parse(args); err != nil {
		return tokenFlags{}, fmt.Errorf("parse flags: %w", err)
	}
	return opts, nil
}

// grantedScopes returns the scopes asked for, the wildcard when none were.
func (o tokenFlags) grantedScopes() apitoken.Scopes {
	if len(o.scopes) == 0 {
		return apitoken.Full()
	}
	return apitoken.Scopes(o.scopes)
}

// lifetime returns the lifetime asked for, the default when unasked.
func (o tokenFlags) lifetime() (time.Duration, error) {
	switch o.ttl {
	case "":
		return defaultTokenLifetime, nil
	case neverWord:
		return apitoken.Never, nil
	}
	days, err := strconv.Atoi(o.ttl)
	if err != nil {
		return 0, fmt.Errorf("parse ttl: %w", err)
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// runTokenVerb runs the named token subcommand against the store.
func runTokenVerb(
	ctx context.Context,
	tokens *postgres.TokenStore,
	userID uuid.UUID,
	verb string,
	opts tokenFlags,
	stdout io.Writer,
) error {
	switch verb {
	case "create":
		return createToken(ctx, tokens, userID, opts, stdout)
	case "list":
		return listTokens(ctx, tokens, userID, stdout)
	case "revoke":
		return revokeToken(ctx, tokens, userID, opts.id, stdout)
	default:
		return fmt.Errorf("token: unknown command %q", verb)
	}
}

// createToken mints a token and prints its secret for the only time.
func createToken(
	ctx context.Context,
	tokens *postgres.TokenStore,
	userID uuid.UUID,
	opts tokenFlags,
	stdout io.Writer,
) error {
	lifetime, err := opts.lifetime()
	if err != nil {
		return err
	}
	minted, err := apitoken.Mint(userID, opts.name, opts.grantedScopes(), lifetime)
	if err != nil {
		return err
	}
	if err := tokens.Create(ctx, minted.Token); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "created token %s\n", minted.Token.ID)
	_, _ = fmt.Fprintf(stdout, "secret: %s\n", minted.Secret)
	_, _ = fmt.Fprintln(stdout, "store it now, it is never shown again")
	_, _ = fmt.Fprintf(stdout, "scopes %s, expires %s\n", minted.Token.Scopes, orNever(minted.Token.ExpiresAt))
	return nil
}

// listTokens prints one line per token of the user, secrets excluded.
func listTokens(ctx context.Context, tokens *postgres.TokenStore, userID uuid.UUID, stdout io.Writer) error {
	stored, err := tokens.ListForUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range stored {
		_, _ = fmt.Fprintf(stdout, "%s  %s  scopes %s  created %s  last used %s  expires %s\n",
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

// revokeToken deletes one token of the user.
func revokeToken(
	ctx context.Context,
	tokens *postgres.TokenStore,
	userID uuid.UUID,
	id string,
	stdout io.Writer,
) error {
	tokenID, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("parse token id: %w", err)
	}
	if err := tokens.Revoke(ctx, userID, tokenID); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "revoked token %s\n", tokenID)
	return nil
}
