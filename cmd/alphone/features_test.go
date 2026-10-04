// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/cucumber/godog"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/apitoken"
	"github.com/gopherium/alphone/internal/role"
)

// operatorCommandsFeature is the feature the command line answers to.
const operatorCommandsFeature = "../../test/features/features/operator-commands.feature"

// wipTag marks the scenarios whose steps are not bound yet.
const wipTag = "@wip"

// operatorScenario is what one operator scenario keeps between its steps.
type operatorScenario struct {
	t      *testing.T
	env    map[string]string
	result testkit.Result
}

// initializeOperatorCommands returns the binding of the operator command steps, t holding their databases.
func initializeOperatorCommands(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &operatorScenario{t: t, env: map[string]string{}}
		sc.Given(`^the settings point at an empty database$`, s.pointAtAnEmptyDatabase)
		sc.Given(`^the settings name no database$`, s.nameNoDatabase)
		sc.When(`^the operator runs alphone with no command$`, s.runWithNoCommand)
		sc.When(`^the operator runs "([^"]*)"$`, s.runLine)
		sc.When(`^the operator asks for the help page of "([^"]*)"$`, s.askForHelp)
		sc.When(`^the operator creates the administrator "([^"]*)" with the password "([^"]*)"$`,
			s.createAdministrator)
		sc.Then(`^the command succeeds$`, s.succeeds)
		sc.Then(`^the command exits with code (\d+)$`, s.exitsWith)
		sc.Then(`^the answer says "([^"]*)" is an unknown command$`, s.namesUnknownCommand)
		sc.Then(`^the answer names the command "([^"]*)"$`, s.namesCommand)
		sc.Then(`^the account "([^"]*)" holds the role "([^"]*)"$`, s.holdsRole)
		sc.Then(`^the answer lists the commands "([^"]*)", "([^"]*)", "([^"]*)" and "([^"]*)"$`, s.listsCommands)
		sc.Then(`^the answer describes "([^"]*)"$`, s.describes)
		sc.Then(`^the answer says nothing changed until it is confirmed with "([^"]*)"$`, s.saysNothingChanged)
		sc.Then(`^the database holds no schema$`, s.holdsNoSchema)
		sc.Then(`^the database holds no demo data$`, s.holdsNoDemoData)
		sc.Then(`^the database holds the demo data$`, s.holdsTheDemoData)
		sc.Given(`^the administrator "([^"]*)"$`, s.holdAdministrator)
		sc.Given(`^the account "([^"]*)" holds a token named "([^"]*)"$`, s.holdToken)
		sc.When(`^the operator mints a token named "([^"]*)" for "([^"]*)"$`, s.mintToken)
		sc.When(`^the operator mints a token named "([^"]*)" for "([^"]*)" with the old spelling "([^"]*)"$`,
			s.mintTokenSpelled)
		sc.When(`^the operator lists the tokens of "([^"]*)" with the old spelling "([^"]*)"$`, s.listTokensSpelled)
		sc.When(`^the operator revokes the token "([^"]*)" of "([^"]*)" with the old spelling "([^"]*)"$`,
			s.revokeTokenSpelled)
		sc.Then(`^the answer carries a secret beginning with "([^"]*)"$`, s.carriesSecret)
		sc.Then(`^the answer says "([^"]*)" is deprecated and names "([^"]*)"$`, s.saysDeprecated)
		sc.Then(`^the answer lists the token "([^"]*)"$`, s.listsToken)
		sc.Then(`^the token list of "([^"]*)" shows "([^"]*)"$`, s.tokenListShows)
		sc.Then(`^the token list of "([^"]*)" shows no secret$`, s.tokenListShowsNoSecret)
		sc.Given(`^the setting "([^"]*)" holds "([^"]*)"$`, s.holdSetting)
		sc.Then(`^the answer names the setting "([^"]*)"$`, s.namesSetting)
	}
}

// holdSetting gives the setting called key the value.
func (s *operatorScenario) holdSetting(key, value string) {
	s.env[key] = value
}

// namesSetting fails unless the refusal names the setting called key.
func (s *operatorScenario) namesSetting(key string) error {
	if !strings.Contains(s.result.Stderr, key) {
		return fmt.Errorf("stderr %q does not name the setting %s", s.result.Stderr, key)
	}
	return nil
}

// holdAdministrator creates the administrator at email, typing the password on standard input.
func (s *operatorScenario) holdAdministrator(email string) error {
	s.createAdministrator(email, typedPassword)
	return s.succeeds()
}

// holdToken mints the token called name for the account at email.
func (s *operatorScenario) holdToken(email, name string) error {
	s.mintToken(name, email)
	return s.succeeds()
}

// mintToken mints the token called name for the account at email.
func (s *operatorScenario) mintToken(name, email string) {
	s.mintTokenSpelled(name, email, "token:create")
}

// mintTokenSpelled mints the token called name for the account at email, the command spelled as spelling.
func (s *operatorScenario) mintTokenSpelled(name, email, spelling string) {
	s.run("", append(strings.Fields(spelling), "-email", email, "-name", name)...)
}

// listTokensSpelled lists the tokens of the account at email, the command spelled as spelling.
func (s *operatorScenario) listTokensSpelled(email, spelling string) {
	s.run("", append(strings.Fields(spelling), "-email", email)...)
}

// revokeTokenSpelled revokes the token called name of the account at email, the command spelled as spelling.
func (s *operatorScenario) revokeTokenSpelled(ctx context.Context, name, email, spelling string) error {
	var id string
	if err := s.scan(ctx, "SELECT id::text FROM core.api_tokens WHERE name = $1", &id, name); err != nil {
		return fmt.Errorf("finding the token %s: %w", name, err)
	}
	s.run("", append(strings.Fields(spelling), "-email", email, "-id", id)...)
	return nil
}

// carriesSecret fails unless the answer prints a secret line starting with prefix.
func (s *operatorScenario) carriesSecret(prefix string) error {
	if !strings.Contains(s.result.Stdout, "\nsecret: "+prefix) {
		return fmt.Errorf("stdout %q carries no secret beginning with %s", s.result.Stdout, prefix)
	}
	return nil
}

// saysDeprecated fails unless the answer says the old spelling is deprecated in favour of the command called name.
func (s *operatorScenario) saysDeprecated(old, name string) error {
	if want := fmt.Sprintf("%q is deprecated, use %q", old, name); !strings.Contains(s.result.Stderr, want) {
		return fmt.Errorf("stderr %q does not carry %q", s.result.Stderr, want)
	}
	return nil
}

// listsToken fails unless the answer lists the token called name.
func (s *operatorScenario) listsToken(name string) error {
	return listing(s.result.Stdout, name)
}

// tokenListShows fails unless token:list for the account at email lists the token called name.
func (s *operatorScenario) tokenListShows(email, name string) error {
	listed, err := s.tokenList(email)
	if err != nil {
		return err
	}
	return listing(listed, name)
}

// tokenListShowsNoSecret fails when token:list for the account at email prints a secret.
func (s *operatorScenario) tokenListShowsNoSecret(email string) error {
	listed, err := s.tokenList(email)
	if err != nil || strings.Contains(listed, apitoken.Prefix) {
		return fmt.Errorf("token:list %q (%v), want no secret listed", listed, err)
	}
	return nil
}

// tokenList returns what token:list prints for the account at email, an error when it fails.
func (s *operatorScenario) tokenList(email string) (string, error) {
	listed := s.answer("", "token:list", "-email", email)
	if listed.Code != 0 {
		return "", fmt.Errorf("token:list exited with %d and stderr %q", listed.Code, listed.Stderr)
	}
	return listed.Stdout, nil
}

// listing fails unless the token list printed holds a line for the token called name.
func listing(printed, name string) error {
	if !strings.Contains(printed, "  "+name+"  scopes ") {
		return fmt.Errorf("the token list %q holds no line for %s", printed, name)
	}
	return nil
}

// pointAtAnEmptyDatabase points the settings at a fresh database holding no schema.
func (s *operatorScenario) pointAtAnEmptyDatabase() {
	s.env["ALPHONE_DATABASE_URL"] = barePostgres(s.t)
}

// nameNoDatabase leaves the database setting out.
func (s *operatorScenario) nameNoDatabase() {
	delete(s.env, "ALPHONE_DATABASE_URL")
}

// answer runs the command line in process over the scenario's settings and the compiled plugins, feeding stdin.
func (s *operatorScenario) answer(stdin string, args ...string) testkit.Result {
	return testkit.Run(s.t, programOver(role.NewRegistry(), testGetenv(s.env), registerPlugins), stdin, args...)
}

// run runs the command line and keeps its answer for the steps that follow.
func (s *operatorScenario) run(stdin string, args ...string) {
	s.result = s.answer(stdin, args...)
}

// runWithNoCommand runs the command line naming no command.
func (s *operatorScenario) runWithNoCommand() {
	s.run("")
}

// runLine runs the command line the text spells, split on its spaces.
func (s *operatorScenario) runLine(line string) {
	s.run("", strings.Fields(line)...)
}

// askForHelp asks for the help page of the command called name.
func (s *operatorScenario) askForHelp(name string) {
	s.run("", "help", name)
}

// createAdministrator creates an administrator at email, typing the password on standard input.
func (s *operatorScenario) createAdministrator(email, password string) {
	s.run(password+"\n", "account:create-admin", "-email", email, "-name", "Administrator", "-role", "admin")
}

// succeeds fails unless the command exited with code 0.
func (s *operatorScenario) succeeds() error {
	return s.exitsWith(0)
}

// exitsWith fails unless the command exited with code.
func (s *operatorScenario) exitsWith(code int) error {
	if s.result.Code != code {
		return fmt.Errorf("the command exited with %d and stderr %q, want %d", s.result.Code, s.result.Stderr, code)
	}
	return nil
}

// namesUnknownCommand fails unless the refusal names the command called name as unknown.
func (s *operatorScenario) namesUnknownCommand(name string) error {
	if !strings.Contains(s.result.Stderr, `unknown command "`+name+`"`) {
		return fmt.Errorf("stderr %q does not name the unknown command %q", s.result.Stderr, name)
	}
	return nil
}

// namesCommand fails unless the refusal names the command the operator can run instead.
func (s *operatorScenario) namesCommand(name string) error {
	if !strings.Contains(s.result.Stderr, name) {
		return fmt.Errorf("stderr %q does not name %q", s.result.Stderr, name)
	}
	return nil
}

// holdsRole fails unless the account at email holds the role.
func (s *operatorScenario) holdsRole(ctx context.Context, email, held string) error {
	var stored bool
	err := s.scan(ctx, "SELECT EXISTS (SELECT FROM auth.users WHERE email = $1 AND role = $2)", &stored, email, held)
	if err != nil || !stored {
		return fmt.Errorf("the account %s holds the role %s %v (%v), want it to", email, held, stored, err)
	}
	return nil
}

// listsCommands fails unless the listing holds a line for each command named.
func (s *operatorScenario) listsCommands(first, second, third, fourth string) error {
	for _, name := range []string{first, second, third, fourth} {
		if !strings.Contains(s.result.Stdout, "\n  "+name+" ") {
			return fmt.Errorf("the listing %q holds no line for %s", s.result.Stdout, name)
		}
	}
	return nil
}

// describes fails unless the answer is the help page of the command called name.
func (s *operatorScenario) describes(name string) error {
	if !strings.Contains(s.result.Stdout, "alphone "+name) {
		return fmt.Errorf("stdout %q is no help page of %s", s.result.Stdout, name)
	}
	return nil
}

// saysNothingChanged fails unless the command reported a dry run that the flag would apply.
func (s *operatorScenario) saysNothingChanged(flag string) error {
	if !strings.Contains(s.result.Stderr, "dry run, nothing changed, pass "+flag+" to apply") {
		return fmt.Errorf("stderr %q reports no dry run that %s applies", s.result.Stderr, flag)
	}
	return nil
}

// holdsNoSchema fails when the database holds a schema beyond the ones every database holds.
func (s *operatorScenario) holdsNoSchema(ctx context.Context) error {
	var schemas []string
	if err := s.scan(ctx, extraSchemasLookup, &schemas); err != nil || len(schemas) > 0 {
		return fmt.Errorf("the database holds the schemas %v (%v), want none", schemas, err)
	}
	return nil
}

// holdsNoDemoData fails when the database holds a contact.
func (s *operatorScenario) holdsNoDemoData(ctx context.Context) error {
	var table bool
	if err := s.scan(ctx, "SELECT to_regclass('core.contacts') IS NOT NULL", &table); err != nil || !table {
		return err
	}
	var stored bool
	if err := s.scan(ctx, "SELECT EXISTS (SELECT FROM core.contacts)", &stored); err != nil || stored {
		return fmt.Errorf("the database holds contacts %v (%v), want none", stored, err)
	}
	return nil
}

// holdsTheDemoData fails unless the database holds the demo admin, contacts and tasks.
func (s *operatorScenario) holdsTheDemoData(ctx context.Context) error {
	var stored bool
	err := s.scan(ctx, "SELECT EXISTS (SELECT FROM auth.users WHERE email = $1) "+
		"AND EXISTS (SELECT FROM core.contacts) AND EXISTS (SELECT FROM core.tasks)", &stored, seedAdminEmail)
	if err != nil || !stored {
		return fmt.Errorf("the database holds the demo data %v (%v), want it stored", stored, err)
	}
	return nil
}

// scan reads the one value query answers on the scenario's database into dest.
func (s *operatorScenario) scan(ctx context.Context, query string, dest any, args ...any) error {
	pool, err := pgxpool.New(ctx, s.env["ALPHONE_DATABASE_URL"])
	if err != nil {
		return err
	}
	defer pool.Close()
	return pool.QueryRow(ctx, query, args...).Scan(dest)
}

func TestOperatorCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping database test in short mode")
	}
	tags := "~" + wipTag
	if os.Getenv("ALPHONE_BDD_WIP") != "" {
		tags = ""
	}
	suite := godog.TestSuite{
		ScenarioInitializer: initializeOperatorCommands(t),
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{operatorCommandsFeature},
			Tags:     tags,
			Strict:   true,
			TestingT: t,
		},
	}
	if suite.Run() != 0 {
		t.Error("the feature did not pass")
	}
}
