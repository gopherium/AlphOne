// SPDX-License-Identifier: Elastic-2.0

// Package email is the email channel plugin.
package email

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/sdk"
)

// adminRole is the role the plugin gives the capability to manage the mailbox.
const adminRole = "admin"

// Plugin is the email channel, storing raw mail on the known contacts it names.
type Plugin struct {
	// pool reaches the mail of every workspace.
	pool *pgxpool.Pool
	// ingester stores and threads each raw mail.
	ingester *ingester
	// sweeper deletes the files no stored mail holds.
	sweeper *sweeper
}

// Register builds the email [Plugin] from the host-provided deps.
func Register(deps sdk.Deps) (*Plugin, error) {
	held, err := readSettings(deps)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.New(context.Background(), deps.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("email: connect database: %w", err)
	}
	return &Plugin{
		pool:     pool,
		ingester: newIngester(pool, deps, held),
		sweeper:  &sweeper{pool: pool, logger: deps.Logger, pace: held.sweep},
	}, nil
}

// ID reports the plugin identifier.
func (p *Plugin) ID() string {
	return "email"
}

// Start launches the sweep of the files no stored mail holds.
func (p *Plugin) Start(_ context.Context) error {
	p.sweeper.Start()
	return nil
}

// Stop ends the sweep and closes the database pool, answering the error of ctx when the sweep overruns it.
func (p *Plugin) Stop(ctx context.Context) error {
	if err := p.sweeper.Stop(ctx); err != nil {
		go p.pool.Close()
		return fmt.Errorf("email: stop the sweep: %w", err)
	}
	p.pool.Close()
	return nil
}

// Migrate creates and updates the plugin-owned plugin_email schema.
func (p *Plugin) Migrate(ctx context.Context) error {
	return migrate(ctx, p.pool)
}

// Roles gives admin the capability to manage the mailbox.
func (p *Plugin) Roles() []sdk.RoleDeclaration {
	return []sdk.RoleDeclaration{{Name: adminRole, Capabilities: []string{manageMailbox}}}
}

// Commands returns the commands the email plugin offers.
func (p *Plugin) Commands() []sdk.Command {
	return []sdk.Command{p.ingester.ingestCommand()}
}

var _ sdk.EventProvider = (*Plugin)(nil)

// Events declares the event names the plugin publishes.
func (p *Plugin) Events() []string {
	return []string{eventMessageReceived}
}

// Seed stores the sample mailbox and its mail in the workspace ctx names, the default one when it names none.
func (p *Plugin) Seed(ctx context.Context) error {
	return p.ingester.seed(ctx, sdk.TenantOrDefault(ctx))
}

var _ sdk.TenantGateConsumer = (*Plugin)(nil)

// UseTenantGate receives the host's tenant gate.
func (p *Plugin) UseTenantGate(gate sdk.TenantGate) {
	p.ingester.gate = gate
}

var _ sdk.FileStoreConsumer = (*Plugin)(nil)

// UseFileStore receives the host's file store, which keeps the files of mail and deletes the ones no mail holds.
func (p *Plugin) UseFileStore(store sdk.FileStore) {
	p.ingester.files.store = store
	p.sweeper.store = store
}

var _ sdk.SettingStoreConsumer = (*Plugin)(nil)

// UseSettingStore receives the host's settings store, which holds a workspace's own mail limit.
func (p *Plugin) UseSettingStore(store sdk.SettingStore) {
	p.ingester.files.settings = store
}
