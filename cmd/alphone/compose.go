// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole"
	accounts "github.com/gopherium/framework/gonsole/auth"
	"github.com/gopherium/framework/pluginkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/contact"
	"github.com/gopherium/alphone/internal/event"
	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/internal/webhook"
	"github.com/gopherium/alphone/sdk"
)

// composeConfig carries the values compose reads.
type composeConfig struct {
	composeSettings
	databaseURL string
	getenv      func(string) string
	roles       *role.Registry
	logger      *slog.Logger
}

// composed is what compose builds over one pool.
type composed struct {
	pool       *pgxpool.Pool
	users      *authkitpg.UserStore
	contacts   *postgres.ContactStore
	tasks      *postgres.TaskStore
	tokens     *postgres.TokenStore
	webhooks   *postgres.WebhookStore
	tenants    *postgres.TenantStore
	worker     *webhook.Worker
	hub        *event.Hub
	events     nudgingPublisher
	mailer     graphres.Mailer
	registered []sdk.Plugin
	failed     error
}

// compose builds the pool, the stores, the events, the mail and the registered plugins, migrating and starting nothing.
func compose(ctx context.Context, cfg composeConfig, plugins func(sdk.Deps) ([]sdk.Plugin, error)) (composed, error) {
	pool, err := pgxpool.New(ctx, cfg.databaseURL)
	if err != nil {
		return composed{}, fmt.Errorf("parse database url: %w", err)
	}
	webhooks := postgres.NewWebhookStore(pool)
	built := composed{
		pool:     pool,
		users:    authkitpg.NewUserStore(pool),
		contacts: postgres.NewContactStore(pool),
		tasks:    postgres.NewTaskStore(pool),
		tokens:   postgres.NewTokenStore(pool),
		webhooks: webhooks,
		tenants:  postgres.NewTenantStore(pool),
		worker:   webhook.NewWorker(webhooks, cfg.logger),
		hub:      event.NewHub(),
	}
	built.events = nudgingPublisher{
		dispatcher: webhook.NewDispatcher(webhooks, cfg.logger), worker: built.worker, hub: built.hub,
	}
	resolver := contact.NewResolver(built.contacts, contact.WithEvents(built.events))
	built.registered, built.failed = plugins(sdk.Deps{
		DatabaseURL:    cfg.databaseURL,
		PublicURL:      cfg.mail.publicURL,
		MachineGrace:   cfg.machineGrace,
		TenantsHeld:    cfg.tenants.held,
		TenantsRefresh: cfg.tenants.refresh,
		Resolver:       resolverBridge{resolver: resolver},
		Contacts:       directoryBridge{resolver: resolver},
		Events:         pluginPublisher{publisher: built.events},
		Getenv:         cfg.getenv,
		Env:            settingsEnv(cfg.getenv),
	})
	if err := declareRoles(cfg.roles, built.registered); err != nil {
		return built, fmt.Errorf("declare plugin roles: %w", err)
	}
	sender, mailer, err := buildMail(cfg.mail, cfg.logger)
	if err != nil {
		return built, err
	}
	built.mailer = mailer
	wireFieldProviders(built.registered)
	wireCredentialProviders(built.registered)
	wireTenantGate(built.registered, tenantGateBridge{tenants: built.tenants, grace: cfg.machineGrace})
	wireMailSenderFrom(built.registered, sender)
	return built, nil
}

// abandon stops the plugins compose registered within grace and closes its pool, nothing when it built no pool.
func abandon(ctx context.Context, built composed, grace time.Duration) error {
	if built.pool == nil {
		return nil
	}
	defer built.pool.Close()
	return gonsole.StopHost(ctx, pluginkit.NewHost(built.registered...), grace)
}

// migrations returns the schema steps every database takes, in the order they apply.
func migrations() []gonsole.Step {
	return []gonsole.Step{accounts.Migration(), {Name: "core", Run: postgres.Migrate}}
}

// migrate applies every schema step to the database at databaseURL.
func migrate(ctx context.Context, databaseURL string) error {
	for _, step := range migrations() {
		if err := step.Run(ctx, databaseURL); err != nil {
			return err
		}
	}
	return nil
}
