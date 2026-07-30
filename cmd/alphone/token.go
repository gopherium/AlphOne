// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/postgres"
)

// token creates, lists, and revokes the API tokens of one user.
func token(ctx context.Context, getenv func(string) string, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("token: want one of create, list, revoke")
	}
	verb := args[0]
	flags := flag.NewFlagSet("token "+verb, flag.ContinueOnError)
	flags.SetOutput(stdout)
	email := flags.String("email", "", "email address of the owning user")
	name := flags.String("name", "", "name of the token to create")
	id := flags.String("id", "", "identifier of the token to revoke")
	if err := flags.Parse(args[1:]); err != nil {
		return fmt.Errorf("parse flags: %w", err)
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

	owner, err := authkitpg.NewUserStore(pool).UserByEmail(ctx, *email)
	if err != nil {
		return err
	}
	tokens := postgres.NewTokenStore(pool)
	switch verb {
	case "create":
		return createToken(ctx, tokens, owner.ID, *name, stdout)
	case "list":
		return listTokens(ctx, tokens, owner.ID, stdout)
	case "revoke":
		return revokeToken(ctx, tokens, owner.ID, *id, stdout)
	default:
		return fmt.Errorf("token: unknown command %q", verb)
	}
}

// createToken mints a token and prints its secret for the only time.
func createToken(
	ctx context.Context,
	tokens *postgres.TokenStore,
	userID uuid.UUID,
	name string,
	stdout io.Writer,
) error {
	minted, err := apitoken.Mint(userID, name)
	if err != nil {
		return err
	}
	if err := tokens.Create(ctx, minted.Token); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(stdout, "created token %s\n", minted.Token.ID)
	_, _ = fmt.Fprintf(stdout, "secret: %s\n", minted.Secret)
	_, _ = fmt.Fprintln(stdout, "store it now, it is never shown again")
	return nil
}

// listTokens prints one line per token of the user, secrets excluded.
func listTokens(ctx context.Context, tokens *postgres.TokenStore, userID uuid.UUID, stdout io.Writer) error {
	stored, err := tokens.ListForUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, t := range stored {
		lastUsed := "never"
		if !t.LastUsedAt.IsZero() {
			lastUsed = t.LastUsedAt.Format("2006-01-02")
		}
		_, _ = fmt.Fprintf(stdout, "%s  %s  created %s  last used %s\n",
			t.ID, t.Name, t.CreatedAt.Format("2006-01-02"), lastUsed)
	}
	return nil
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
