// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/gouncer"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/plugins/whatsapp"
	"github.com/gopherium/alphone/sdk"
)

// Demo credentials stored by the seed subcommand, for development only.
const (
	seedAdminEmail    = "admin@example.com"
	seedAdminName     = "Admin"
	seedAdminPassword = "password1234"
)

// seed migrates the database and stores the demo data set.
func seed(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	databaseURL := getenv("ALPHONE_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("ALPHONE_DATABASE_URL is required")
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("parse database url: %w", err)
	}
	defer pool.Close()
	if err := authkitpg.Migrate(ctx, databaseURL); err != nil {
		return err
	}
	if err := postgres.Migrate(ctx, databaseURL); err != nil {
		return err
	}
	created, err := seedAdmin(ctx, authkitpg.NewUserStore(pool), seedAdminEmail, seedAdminName, seedAdminPassword)
	if err != nil {
		return err
	}
	resolver := contact.NewResolver(postgres.NewContactStore(pool))
	if _, err := resolver.Resolve(ctx, "email", "ada@example.com", "Ada Lovelace"); err != nil {
		return fmt.Errorf("seed contact: %w", err)
	}
	if err := seedWhatsApp(ctx, databaseURL, getenv, resolver); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(stdout, "seeded demo data")
	if created {
		_, _ = fmt.Fprintln(stdout, "login: "+seedAdminEmail+" / "+seedAdminPassword)
	} else {
		_, _ = fmt.Fprintln(stdout, seedAdminEmail+" already exists, its password is unchanged")
	}
	_, _ = fmt.Fprintln(stdout, "development only, never seed a production database")
	return nil
}

// seedAdmin creates the demo admin account, reporting whether it was newly created.
func seedAdmin(ctx context.Context, store gouncer.Store, email, name, password string) (bool, error) {
	admin, err := gouncer.NewUser(email, name, password)
	if err != nil {
		return false, fmt.Errorf("build admin: %w", err)
	}
	err = store.CreateUser(ctx, admin)
	if errors.Is(err, gouncer.ErrEmailTaken) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create admin: %w", err)
	}
	return true, nil
}

// seedWhatsApp registers the WhatsApp plugin and stores its demo conversations.
func seedWhatsApp(
	ctx context.Context,
	databaseURL string,
	getenv func(string) string,
	resolver *contact.Resolver,
) error {
	p, err := whatsapp.Register(sdk.Deps{
		DatabaseURL: databaseURL,
		Resolver:    resolverBridge{resolver: resolver},
		Getenv:      getenv,
	})
	if err != nil {
		return err
	}
	defer func() { _ = p.Stop(context.WithoutCancel(ctx)) }()
	if err := p.Migrate(ctx); err != nil {
		return err
	}
	return p.Seed(ctx)
}
