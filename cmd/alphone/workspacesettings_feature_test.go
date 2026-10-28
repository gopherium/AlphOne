// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// workspaceSettingsFeature is the feature the settings stores answer to.
const workspaceSettingsFeature = "../../test/features/features/workspace-settings.feature"

// settingsPluginID names the mail plugin the workspace settings scenarios run.
const settingsPluginID = "email"

// calendarPluginID names the calendar plugin the workspace settings scenarios run beside the mail plugin.
const calendarPluginID = "calendar"

// mailboxHostKey is the key the plugin keeps the mailbox host under.
const mailboxHostKey = "imap.host"

// mailboxPasswordKey is the key the plugin keeps the mailbox password under.
const mailboxPasswordKey = "imap.password"

// copyStoredSecret stores the encrypted value of one key of one workspace under another workspace, by hand.
const copyStoredSecret = `INSERT INTO core.settings (tenant_id, key, encrypted)
	SELECT $1, key, encrypted FROM core.settings WHERE tenant_id = $2 AND key = $3`

// storeHandingOutStores is a settings store that hands out the store of any plugin.
type storeHandingOutStores interface {
	ForPlugin(id string) sdk.SettingStore
}

// settingsScenario is what one workspace settings scenario keeps between its steps.
type settingsScenario struct {
	t          *testing.T
	env        map[string]string
	pool       *pgxpool.Pool
	workspaces map[string]uuid.UUID
	mail       *settingTakingPlugin
	calendar   *settingTakingPlugin
	reached    sdk.SettingStore
	built      composed
	composed   bool
}

// initializeWorkspaceSettings returns the binding of the workspace settings steps, t holding their databases.
func initializeWorkspaceSettings(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &settingsScenario{t: t, env: map[string]string{}, workspaces: map[string]uuid.UUID{}}
		sc.Given(`^the database is migrated$`, s.migrateTheDatabase)
		sc.Given(`^the server holds an encryption key$`, s.holdAnEncryptionKey)
		sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
		sc.Given(`^AlphOne runs a mail plugin and a calendar plugin that keep settings$`, s.runPluginsThatKeepSettings)
		sc.When(`^the workspace "([^"]*)" saves the mailbox host "([^"]*)"$`, s.saveHost)
		sc.Then(`^the workspace "([^"]*)" reads the mailbox host "([^"]*)"$`, s.readsHost)
		sc.Given(`^the workspace "([^"]*)" saves the mailbox password "([^"]*)"$`, s.savePassword)
		sc.When(`^the mailbox password of "([^"]*)" is copied to "([^"]*)"$`, s.copyPassword)
		sc.Then(`^the workspace "([^"]*)" cannot open its mailbox password$`, s.cannotOpenPassword)
		sc.Then(`^the workspace "([^"]*)" still opens its mailbox password as "([^"]*)"$`, s.opensPassword)
		sc.When(`^the calendar plugin asks its store for the store of the mail plugin$`, s.askForTheMailStore)
		sc.Then(`^the calendar plugin gets no store of the mail plugin$`, s.getsNoMailStore)
		sc.Then(`^the calendar plugin reads no mailbox password in "([^"]*)"$`, s.readsNoPasswordIn)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop(ctx)
		})
	}
}

// migrateTheDatabase points the settings at a fresh database every schema step migrated.
func (s *settingsScenario) migrateTheDatabase(ctx context.Context) error {
	databaseURL := barePostgres(s.t)
	if err := migrate(ctx, databaseURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	s.env["ALPHONE_DATABASE_URL"], s.pool = databaseURL, pool
	return nil
}

// holdAnEncryptionKey sets the key the server encrypts secrets with.
func (s *settingsScenario) holdAnEncryptionKey() {
	s.env["ALPHONE_ENCRYPTION_KEY"] = currentEncryptionKey
}

// createWorkspace stores the workspace called name and keeps its id.
func (s *settingsScenario) createWorkspace(ctx context.Context, name string) error {
	id := uuid.Must(uuid.NewV7())
	if _, err := s.pool.Exec(ctx, "INSERT INTO core.tenants (id, name) VALUES ($1, $2)", id, name); err != nil {
		return fmt.Errorf("storing the workspace %s: %w", name, err)
	}
	s.workspaces[name] = id
	return nil
}

// runPluginsThatKeepSettings composes the host over the settings with a mail and a calendar plugin taking a store.
func (s *settingsScenario) runPluginsThatKeepSettings(ctx context.Context) error {
	settings, err := loadRunConfig(testkit.Getenv(s.env))
	if err != nil {
		return err
	}
	s.mail = &settingTakingPlugin{inertPlugin: inertPlugin{id: settingsPluginID}}
	s.calendar = &settingTakingPlugin{inertPlugin: inertPlugin{id: calendarPluginID}}
	s.built, err = compose(ctx, composeConfig{
		composeSettings: settings.composeSettings,
		databaseURL:     settings.databaseURL,
		getenv:          testkit.Getenv(s.env),
		roles:           role.NewRegistry(),
		logger:          discardLogger(),
	}, func(sdk.Deps) ([]sdk.Plugin, error) { return []sdk.Plugin{s.mail, s.calendar}, nil })
	s.composed = true
	if err != nil {
		return err
	}
	for _, plugin := range []*settingTakingPlugin{s.mail, s.calendar} {
		if plugin.settings == nil {
			return fmt.Errorf("the plugin %s received no settings store", plugin.id)
		}
	}
	return nil
}

// in returns ctx serving the workspace called name.
func (s *settingsScenario) in(ctx context.Context, name string) context.Context {
	return sdk.WithTenant(ctx, s.workspaces[name])
}

// saveHost saves the mailbox host of the workspace called name.
func (s *settingsScenario) saveHost(ctx context.Context, name, host string) error {
	return s.mail.settings.Save(s.in(ctx, name), map[string]string{mailboxHostKey: host})
}

// readsHost fails unless the workspace called name reads host as its mailbox host.
func (s *settingsScenario) readsHost(ctx context.Context, name, host string) error {
	if got := readPlain(s.in(ctx, name), s.mail.settings, mailboxHostKey); got != foundValue(host) {
		return fmt.Errorf("the mailbox host of %s reads %+v, want %q", name, got, host)
	}
	return nil
}

// savePassword saves the mailbox password of the workspace called name as a secret.
func (s *settingsScenario) savePassword(ctx context.Context, name, password string) error {
	return s.mail.settings.SaveSecrets(s.in(ctx, name), map[string]string{mailboxPasswordKey: password})
}

// copyPassword stores what the workspace called from holds encrypted as its mailbox password in the one called to.
func (s *settingsScenario) copyPassword(ctx context.Context, from, to string) error {
	stored := "plugin." + settingsPluginID + "." + mailboxPasswordKey
	if _, err := s.pool.Exec(ctx, copyStoredSecret, s.workspaces[to], s.workspaces[from], stored); err != nil {
		return fmt.Errorf("copying the mailbox password from %s to %s: %w", from, to, err)
	}
	return nil
}

// cannotOpenPassword fails unless the workspace called name finds its mailbox password and cannot decrypt it.
func (s *settingsScenario) cannotOpenPassword(ctx context.Context, name string) error {
	got := readSecret(s.in(ctx, name), s.mail.settings, mailboxPasswordKey)
	if !errors.Is(got.err, sdk.ErrCannotDecrypt) || got.found {
		return fmt.Errorf("the mailbox password of %s reads %+v, want nothing beside %v", name, got,
			sdk.ErrCannotDecrypt)
	}
	return nil
}

// opensPassword fails unless the workspace called name opens password as its mailbox password.
func (s *settingsScenario) opensPassword(ctx context.Context, name, password string) error {
	if got := readSecret(s.in(ctx, name), s.mail.settings, mailboxPasswordKey); got != foundValue(password) {
		return fmt.Errorf("the mailbox password of %s reads %+v, want %q", name, got, password)
	}
	return nil
}

// askForTheMailStore keeps the store of the mail plugin that the calendar plugin's store hands out, if it hands any.
func (s *settingsScenario) askForTheMailStore() {
	if handing, ok := s.calendar.settings.(storeHandingOutStores); ok {
		s.reached = handing.ForPlugin(settingsPluginID)
	}
}

// getsNoMailStore fails when the calendar plugin reached the store of the mail plugin through its own store.
func (s *settingsScenario) getsNoMailStore() error {
	if s.reached != nil {
		return errors.New("the calendar plugin reached the store of the mail plugin, want its own store alone")
	}
	return nil
}

// readsNoPasswordIn fails unless the calendar plugin reads no mailbox password in the workspace called name.
func (s *settingsScenario) readsNoPasswordIn(ctx context.Context, name string) error {
	if got := readSecret(s.in(ctx, name), s.calendar.settings, mailboxPasswordKey); got != (settingRead{}) {
		return fmt.Errorf("the calendar plugin reads the mailbox password of %s as %+v, want it unset", name, got)
	}
	return nil
}

// stop stops what the scenario composed and closes its pool.
func (s *settingsScenario) stop(ctx context.Context) error {
	var err error
	if s.composed {
		err = abandon(ctx, s.built, pluginStopGrace)
	}
	if s.pool != nil {
		s.pool.Close()
	}
	return err
}

func TestWorkspaceSettings(t *testing.T) {
	runFeature(t, workspaceSettingsFeature, initializeWorkspaceSettings(t))
}
