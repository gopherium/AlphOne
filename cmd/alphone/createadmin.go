// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"

	"github.com/gopherium/alphone/internal/postgres"
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
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}

	_, _ = fmt.Fprint(stdout, "Password: ")
	scanner := bufio.NewScanner(stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read password: %w", err)
		}
		return errors.New("read password: no input")
	}

	u, err := gouncer.NewUser(*email, *name, scanner.Text())
	if err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := postgres.Migrate(ctx, databaseURL); err != nil {
		return err
	}
	if err := postgres.NewUserStore(pool).CreateUser(ctx, u); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "created user %s\n", u.Email)
	return nil
}
