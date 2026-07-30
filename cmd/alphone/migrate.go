// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"

	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/postgres"
)

// migrateSchemas applies the auth and core schemas in order.
func migrateSchemas(ctx context.Context, databaseURL string) error {
	for _, migrate := range []func(context.Context, string) error{authkitpg.Migrate, postgres.Migrate} {
		if err := migrate(ctx, databaseURL); err != nil {
			return err
		}
	}
	return nil
}
