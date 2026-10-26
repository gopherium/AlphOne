// SPDX-License-Identifier: Elastic-2.0

package storage

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/gopherium/framework/postgresblob"
)

// Migrate creates the storage schema and its tables in the database at databaseURL.
func Migrate(ctx context.Context, databaseURL string) error {
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return fmt.Errorf("storage: read the database address: %w", err)
	}
	db := stdlib.OpenDB(*config)
	defer func() { _ = db.Close() }()
	if err := postgresblob.Migrate(ctx, db, Schema); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	return nil
}
