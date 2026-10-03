// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// extraSchemasLookup lists the schemas of the database beyond the ones every database holds.
const extraSchemasLookup = "SELECT coalesce(array_agg(nspname ORDER BY nspname), '{}') FROM pg_namespace " +
	"WHERE nspname NOT IN ('public', 'information_schema') AND nspname NOT LIKE 'pg\\_%'"

// startRecordingPlugin records whether the host started it.
type startRecordingPlugin struct {
	inertPlugin
	started atomic.Bool
}

// Start records the start.
func (p *startRecordingPlugin) Start(context.Context) error {
	p.started.Store(true)
	return nil
}

// fieldServingPlugin offers the contact fields of a plugin that defines them.
type fieldServingPlugin struct{ inertPlugin }

// LiveContactFields answers no field.
func (fieldServingPlugin) LiveContactFields(context.Context) ([]sdk.ContactField, error) {
	return nil, nil
}

// CheckContactFieldTexts accepts every text.
func (fieldServingPlugin) CheckContactFieldTexts(context.Context, map[string]string) error {
	return nil
}

// WriteContactFieldTexts stores nothing.
func (fieldServingPlugin) WriteContactFieldTexts(context.Context, uuid.UUID, map[string]string) error {
	return nil
}

// fieldTakingPlugin records the field providers the host hands it.
type fieldTakingPlugin struct {
	inertPlugin
	received []sdk.FieldProvider
}

// UseFieldProviders keeps the handed providers.
func (p *fieldTakingPlugin) UseFieldProviders(providers []sdk.FieldProvider) {
	p.received = providers
}

// registeringNothing registers no plugin.
func registeringNothing(sdk.Deps) ([]sdk.Plugin, error) {
	return nil, nil
}

// partlyRegisteredWithARefusedRole registers one plugin declaring a role with no name beside a registration failure.
func partlyRegisteredWithARefusedRole(sdk.Deps) ([]sdk.Plugin, error) {
	return []sdk.Plugin{rolePlugin{declared: []sdk.RoleDeclaration{{Name: ""}}}}, errRegistration
}

// filledSettings returns every core setting filled over an unreachable database, a relay the mail sender accepts.
func filledSettings() map[string]string {
	env := coreSettings()
	env["ALPHONE_DATABASE_URL"] = unreachableDatabaseURL
	env["ALPHONE_SMTP_TLS"] = "mandatory"
	delete(env, "ALPHONE_MAIL_TEMPLATE_DIR")
	return env
}

// composeConfigOf returns what compose reads over env, the roles declared into a registry of the test's own.
func composeConfigOf(t *testing.T, env map[string]string) composeConfig {
	t.Helper()
	settings, err := loadRunConfig(testGetenv(env))
	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	return composeConfig{
		composeSettings: settings.composeSettings,
		databaseURL:     settings.databaseURL,
		getenv:          testGetenv(env),
		roles:           role.NewRegistry(),
		logger:          discardLogger(),
	}
}

// abandonAtEnd stops what compose registered and closes its pool once the test ends.
func abandonAtEnd(t *testing.T, built composed) {
	t.Helper()
	t.Cleanup(func() {
		if err := abandon(context.Background(), built, pluginStopGrace); err != nil {
			t.Errorf("abandon() error = %v, want nil", err)
		}
	})
}

// extraSchemas returns the schemas of the database at address beyond the ones every database holds.
func extraSchemas(t *testing.T, address string) []string {
	t.Helper()
	var schemas []string
	if err := sessionTo(t, address).QueryRow(t.Context(), extraSchemasLookup).Scan(&schemas); err != nil {
		t.Fatalf("listing the schemas: %v", err)
	}
	return schemas
}

func TestComposeRegistersThePluginsWithoutMigratingOrStartingThem(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)
	recording := &startRecordingPlugin{inertPlugin: inertPlugin{id: "recording"}}

	built, err := compose(t.Context(), composeConfigOf(t, map[string]string{"ALPHONE_DATABASE_URL": databaseURL}),
		besideTheCompiledPlugins(recording))

	if err != nil || built.failed != nil {
		t.Fatalf("compose() = %v with %v failed, want every plugin registered", err, built.failed)
	}
	abandonAtEnd(t, built)
	if ids := idsOf(built.registered); !slices.Contains(ids, "fields") || !slices.Contains(ids, "recording") {
		t.Errorf("registered = %v, want the compiled plugins and the recording one among them", ids)
	}
	if recording.started.Load() {
		t.Error("the recording plugin was started, want compose to start nothing")
	}
	if schemas := extraSchemas(t, databaseURL); len(schemas) > 0 {
		t.Errorf("the database holds the schemas %v after compose, want nothing migrated", schemas)
	}
}

func TestComposeHandsThePluginsEveryDependency(t *testing.T) {
	t.Parallel()

	var handed sdk.Deps

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), capturingPlugins(&handed))

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	deps := reflect.ValueOf(handed)
	for i := range deps.NumField() {
		if deps.Field(i).IsZero() {
			t.Errorf("Deps.%s is zero, want compose to hand the plugins every dependency", deps.Type().Field(i).Name)
		}
	}
}

func TestComposeWiresTheFieldProviders(t *testing.T) {
	t.Parallel()

	serving := fieldServingPlugin{inertPlugin{id: "serving"}}
	taking := &fieldTakingPlugin{inertPlugin: inertPlugin{id: "taking"}}

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{serving, taking}, nil
	})

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	if len(taking.received) != 1 || taking.received[0] != sdk.FieldProvider(serving) {
		t.Errorf("the taking plugin received %v, want the serving plugin as its one field provider", taking.received)
	}
}

func TestComposeWiresTheCredentialsTheTenantGateAndTheMailSender(t *testing.T) {
	t.Parallel()

	serving := credentialServingPlugin{inertPlugin{id: "serving"}}
	credentials := &credentialTakingPlugin{inertPlugin: inertPlugin{id: "credentials"}}
	gated := &gateTakingPlugin{inertPlugin: inertPlugin{id: "gated"}}
	mailing := &mailTakingPlugin{inertPlugin: inertPlugin{id: "mailing"}}
	cfg := composeConfigOf(t, filledSettings())

	built, err := compose(t.Context(), cfg, func(sdk.Deps) ([]sdk.Plugin, error) {
		return []sdk.Plugin{serving, credentials, gated, mailing}, nil
	})

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	if len(credentials.received) != 1 || credentials.received[0] != sdk.CredentialProvider(serving) {
		t.Errorf("the credentials plugin received %v, want the serving plugin", credentials.received)
	}
	if want := (tenantGateBridge{tenants: built.tenants, grace: cfg.machineGrace}); gated.received != want {
		t.Errorf("the gated plugin received %#v, want the gate over the composed tenants and the machine grace",
			gated.received)
	}
	if !mailing.handed || mailing.received == nil {
		t.Error("the mailing plugin received no sender, want the relay the settings name")
	}
}

func TestComposeInDescribeModeNeedsNoDatabaseSetting(t *testing.T) {
	t.Parallel()

	cfg := composeConfig{getenv: testGetenv(nil), roles: role.NewRegistry(), logger: discardLogger()}

	built, err := compose(t.Context(), cfg, registerPlugins)

	if err != nil || built.failed != nil {
		t.Fatalf("compose() = %v with %v failed, want the plugins registered without a database setting",
			err, built.failed)
	}
	abandonAtEnd(t, built)
	if ids := idsOf(built.registered); !slices.Contains(ids, "fields") {
		t.Errorf("registered = %v, want the compiled plugins among them", ids)
	}
}

func TestComposeLeavesTheWebhookWorkerStopped(t *testing.T) {
	t.Parallel()

	var logged strings.Builder
	cfg := composeConfigOf(t, filledSettings())
	cfg.logger = slog.New(slog.NewTextHandler(&logged, nil))

	built, err := compose(t.Context(), cfg, registeringNothing)

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	built.worker.Stop()
	if strings.Contains(logged.String(), "claiming webhook deliveries") {
		t.Errorf("the webhook worker swept the queue, want compose to start no worker, logged:\n%s", logged.String())
	}
}

func TestAbandonClosesThePoolComposeOpened(t *testing.T) {
	t.Parallel()

	built, err := compose(t.Context(), composeConfigOf(t, filledSettings()), registeringNothing)
	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}

	if err := abandon(t.Context(), built, pluginStopGrace); err != nil {
		t.Fatalf("abandon() error = %v, want nil", err)
	}

	if err := built.pool.Ping(t.Context()); err == nil || !strings.Contains(err.Error(), "closed pool") {
		t.Errorf("Ping() after abandon error = %v, want the closed pool refused", err)
	}
}

func TestMigrateAppliesEveryStepToABareDatabase(t *testing.T) {
	t.Parallel()

	databaseURL := barePostgres(t)

	err := migrate(t.Context(), databaseURL)

	if err != nil {
		t.Fatalf("migrate() error = %v, want the accounts step applied before the core step needs it", err)
	}
	if schemas := extraSchemas(t, databaseURL); !slices.Contains(schemas, "auth") || !slices.Contains(schemas, "core") {
		t.Errorf("the database holds the schemas %v after migrate, want the auth and core schemas", schemas)
	}
}

func TestRunNamesTheRegistrationFailureBesideARefusedRole(t *testing.T) {
	t.Parallel()

	getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	err := run(t.Context(), getenv, io.Discard, partlyRegisteredWithARefusedRole)

	if !errors.Is(err, errRegistration) || !errors.Is(err, role.ErrEmptyRole) {
		t.Errorf("run() error = %v, want the registration failure and the refused role both named", err)
	}
}
