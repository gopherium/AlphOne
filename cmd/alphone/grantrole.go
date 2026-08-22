// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/role"
)

// grantRole gives a role to every account holding none, from command-line arguments.
func grantRole(ctx context.Context, getenv func(string) string, args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("grantrole", flag.ContinueOnError)
	flags.SetOutput(stdout)
	named := flags.String("role", "", "role to give every account holding none")
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
	if err := migrateSchemas(ctx, databaseURL); err != nil {
		return err
	}
	return authkitpg.RunGrantRole(ctx, databaseURL, []string{"-role", held.String()}, stdout)
}
