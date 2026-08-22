// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
)

// createAdmin provisions a user account from the command line, reading
// the password as one line from stdin.
func createAdmin(
	ctx context.Context,
	getenv func(string) string,
	args []string,
	stdin io.Reader,
	stdout io.Writer,
) error {
	flags := flag.NewFlagSet("createadmin", flag.ContinueOnError)
	flags.SetOutput(stdout)
	email := flags.String("email", "", "email address of the new user")
	name := flags.String("name", "", "display name of the new user")
	named := flags.String("role", "", "role the new user starts under")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("parse flags: %w", err)
	}

	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	held, err := role.Parse(*named)
	if err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := migrateSchemas(ctx, databaseURL); err != nil {
		return err
	}

	users := authkitpg.NewUserStore(pool)
	if err := authkit.CreateAdmin(ctx, users, *email, *name, held.String(), stdin, stdout); err != nil {
		return err
	}
	return grantRole(ctx, pool, users, *email, held)
}

// grantRole puts the named user in the given tier.
func grantRole(
	ctx context.Context,
	pool *pgxpool.Pool,
	users *authkitpg.UserStore,
	email string,
	held role.Role,
) error {
	owner, err := users.UserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return err
	}
	return postgres.NewRoleStore(pool).Grant(ctx, owner.ID, held)
}
