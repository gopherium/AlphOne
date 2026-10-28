// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/google/uuid"

	"github.com/gopherium/framework/gonsole/testkit"
	"github.com/gopherium/framework/pluginkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// whatsAppTokensFeature is the feature the WhatsApp tokens answer to.
const whatsAppTokensFeature = "../../test/features/features/whatsapp-tokens.feature"

// whatsAppChannel names the channel the WhatsApp plugin offers its credentials on.
const whatsAppChannel sdk.Channel = "whatsapp"

// defaultWorkspace is the name the scenarios give the workspace every install starts with.
const defaultWorkspace = "the default workspace"

// copiedNumber is the number a copied token is stored under.
const copiedNumber = "5550002"

// copyStoredToken stores the encrypted token of one workspace under another workspace and number, by hand.
const copyStoredToken = `INSERT INTO plugin_whatsapp.credentials (tenant_id, phone_number_id, access_token)
	SELECT $1, $2, access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $3`

// workspaceAccessToken reads the encrypted token one workspace holds.
const workspaceAccessToken = "SELECT access_token FROM plugin_whatsapp.credentials WHERE tenant_id = $1"

// workspaceAccessTokens counts the tokens every workspace holds.
const workspaceAccessTokens = "SELECT count(*) FROM plugin_whatsapp.credentials"

// tokensScenario is what one WhatsApp token scenario keeps between its steps.
type tokensScenario struct {
	settingsScenario
	whatsApp sdk.CredentialProvider
	attempt  error
}

// initializeWhatsAppTokens returns the binding of the WhatsApp token steps, t holding their databases.
func initializeWhatsAppTokens(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &tokensScenario{settingsScenario: settingsScenario{
			t: t, env: map[string]string{}, workspaces: map[string]uuid.UUID{defaultWorkspace: sdk.DefaultTenantID},
		}}
		sc.Given(`^the database is migrated$`, s.migrateTheDatabase)
		sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
		sc.Given(`^the server holds an encryption key$`, s.holdAnEncryptionKey)
		sc.Given(`^the environment file names the WhatsApp number "([^"]*)" with its token$`, s.nameTheNumber)
		sc.Given(`^AlphOne runs its plugins$`, s.runThePlugins)
		sc.Step(`^the workspace "([^"]*)" stores the WhatsApp number "([^"]*)" with its token$`, s.store)
		sc.When(`^the workspace "([^"]*)" tries to store the WhatsApp number "([^"]*)" with its token$`, s.tryToStore)
		sc.When(`^the token of "([^"]*)" is copied to "([^"]*)" under another number$`, s.copyToken)
		sc.Then(`^the workspace "([^"]*)" answers the WhatsApp number "([^"]*)"$`, s.answersNumber)
		sc.Then(`^the default workspace answers the WhatsApp number "([^"]*)"$`, s.defaultAnswersNumber)
		sc.Then(`^the token stored for "([^"]*)" is not readable at rest$`, s.storedTokenHides)
		sc.Then(`^the workspace "([^"]*)" cannot open its WhatsApp token$`, s.cannotDecryptToken)
		sc.Then(`^the attempt answers an error that names "([^"]*)"$`, s.attemptNames)
		sc.Then(`^no WhatsApp token is stored$`, s.storesNoToken)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop(ctx)
		})
	}
}

// tokenOf answers the access token the workspace called name stores.
func tokenOf(name string) string {
	return "EAAG-" + strings.ToLower(name) + "-token"
}

// nameTheNumber sets the number and token the environment file names for the default workspace.
func (s *tokensScenario) nameTheNumber(number string) {
	s.env["ALPHONE_WHATSAPP_PHONE_NUMBER_ID"] = number
	s.env["ALPHONE_WHATSAPP_ACCESS_TOKEN"] = tokenOf(defaultWorkspace)
}

// runThePlugins composes the host over the settings with every compiled plugin and migrates them.
func (s *tokensScenario) runThePlugins(ctx context.Context) error {
	settings, err := loadRunConfig(testkit.Getenv(s.env))
	if err != nil {
		return err
	}
	s.built, err = compose(ctx, composeConfig{
		composeSettings: settings.composeSettings,
		databaseURL:     settings.databaseURL,
		getenv:          testkit.Getenv(s.env),
		roles:           role.NewRegistry(),
		logger:          discardLogger(),
	}, registerPlugins)
	s.composed = true
	if err != nil {
		return err
	}
	if err := pluginkit.NewHost(s.built.registered...).Migrate(ctx); err != nil {
		return err
	}
	return s.findWhatsApp()
}

// findWhatsApp keeps the compiled plugin that offers WhatsApp credentials.
func (s *tokensScenario) findWhatsApp() error {
	for _, plugin := range s.built.registered {
		if provider, offers := plugin.(sdk.CredentialProvider); offers && provider.Channel() == whatsAppChannel {
			s.whatsApp = provider
			return nil
		}
	}
	return errors.New("no compiled plugin offers WhatsApp credentials")
}

// store stores the number of the workspace called name with its token.
func (s *tokensScenario) store(ctx context.Context, name, number string) error {
	return s.whatsApp.SetChannelCredentials(s.in(ctx, name), number, tokenOf(name))
}

// tryToStore tries to store the number of the workspace called name with its token and keeps what it answered.
func (s *tokensScenario) tryToStore(ctx context.Context, name, number string) {
	s.attempt = s.store(ctx, name, number)
}

// copyToken stores what the workspace called from holds encrypted as its token in the one called to.
func (s *tokensScenario) copyToken(ctx context.Context, from, to string) error {
	if _, err := s.pool.Exec(ctx, copyStoredToken, s.workspaces[to], copiedNumber, s.workspaces[from]); err != nil {
		return fmt.Errorf("copying the token from %s to %s: %w", from, to, err)
	}
	return nil
}

// numberIn answers what the WhatsApp plugin reads as the number of the workspace ctx serves.
func (s *tokensScenario) numberIn(ctx context.Context) settingRead {
	identifier, configured, err := s.whatsApp.ChannelIdentifier(ctx)
	return settingRead{value: identifier, found: configured, err: err}
}

// answersNumber fails unless the workspace called name opens its token and answers number.
func (s *tokensScenario) answersNumber(ctx context.Context, name, number string) error {
	if got := s.numberIn(s.in(ctx, name)); got != foundValue(number) {
		return fmt.Errorf("the WhatsApp number of %s reads %+v, want %q", name, got, number)
	}
	return nil
}

// defaultAnswersNumber fails unless the default workspace answers number.
func (s *tokensScenario) defaultAnswersNumber(ctx context.Context, number string) error {
	return s.answersNumber(ctx, defaultWorkspace, number)
}

// storedTokenHides fails unless the bytes stored as the token of the workspace called name do not hold its token.
func (s *tokensScenario) storedTokenHides(ctx context.Context, name string) error {
	var stored []byte
	if err := s.pool.QueryRow(ctx, workspaceAccessToken, s.workspaces[name]).Scan(&stored); err != nil {
		return fmt.Errorf("reading the token of %s: %w", name, err)
	}
	if bytes.Contains(stored, []byte(tokenOf(name))) {
		return fmt.Errorf("the token of %s is readable at rest, want it encrypted", name)
	}
	return nil
}

// cannotDecryptToken fails unless the workspace called name finds its token and cannot decrypt it.
func (s *tokensScenario) cannotDecryptToken(ctx context.Context, name string) error {
	if got := s.numberIn(s.in(ctx, name)); !errors.Is(got.err, sdk.ErrCannotDecrypt) || got.found {
		return fmt.Errorf("the WhatsApp number of %s reads %+v, want nothing beside %v", name, got, sdk.ErrCannotDecrypt)
	}
	return nil
}

// attemptNames fails unless the last attempt answered the missing key error naming variable.
func (s *tokensScenario) attemptNames(variable string) error {
	if !errors.Is(s.attempt, sdk.ErrNoEncryptionKey) || !strings.Contains(fmt.Sprint(s.attempt), variable) {
		return fmt.Errorf("the attempt answered %v, want %v naming %s", s.attempt, sdk.ErrNoEncryptionKey, variable)
	}
	return nil
}

// storesNoToken fails unless no workspace holds a WhatsApp token.
func (s *tokensScenario) storesNoToken(ctx context.Context) error {
	var stored int
	if err := s.pool.QueryRow(ctx, workspaceAccessTokens).Scan(&stored); err != nil || stored != 0 {
		return fmt.Errorf("the plugin holds %d tokens (%v), want none", stored, err)
	}
	return nil
}

func TestWhatsAppTokens(t *testing.T) {
	runFeature(t, whatsAppTokensFeature, initializeWhatsAppTokens(t))
}
