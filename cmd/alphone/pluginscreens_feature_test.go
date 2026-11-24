// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/cucumber/godog"

	"github.com/gopherium/framework/gonsole/testkit"
)

// pluginScreensFeature is the feature hiding the screens of a plugin answers to.
const pluginScreensFeature = "../../test/features/features/plugin-screens.feature"

// hiddenPluginsField is the root field that names the plugins hiding their screens in the caller's workspace.
const hiddenPluginsField = "hiddenPlugins"

// hiddenPluginsQuery reads the ids of the plugins that hide their screens in the caller's workspace.
const hiddenPluginsQuery = `query { hiddenPlugins }`

// unauthenticatedCode is the code the graph answers a caller who is not signed in.
const unauthenticatedCode = "UNAUTHENTICATED"

// screensScenario is what one plugin screens scenario keeps between its steps, beside what the read steps keep.
type screensScenario struct {
	*readScenario
	logged  lockedLog
	stopped error
}

// initializePluginScreens returns the binding of the plugin screens steps, t holding their databases.
func initializePluginScreens(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &screensScenario{readScenario: newReadScenario(newEmailScenario(t))}
		sc.Given(`^the database is migrated$`, s.migrateTheDatabase)
		sc.Given(`^the setting "([^"]*)" holds "([^"]*)"$`, s.holdSetting)
		sc.Given(`^the administrator "([^"]*)"$`, s.holdAdministrator)
		sc.Given(`^the member "([^"]*)"$`, s.holdMember)
		sc.Given(`^the workspace "([^"]*)" exists$`, s.createWorkspace)
		sc.Given(`^the account "([^"]*)" is placed in the workspace "([^"]*)"$`, s.placeAccount)
		sc.Given(`^the member "([^"]*)" holds a token scoped to "([^"]*)"$`, s.holdToken)
		sc.Given(`^the workspace "([^"]*)" knows "([^"]*)" at "([^"]*)"$`, s.knowContact)
		sc.Given(`^this mail reached "([^"]*)":$`, s.reachOrFail)
		sc.When(`^the member "([^"]*)" reads which plugins hide their screens$`, s.readHidden)
		sc.When(`^a caller who is not signed in reads which plugins hide their screens$`, s.readHiddenUnsigned)
		sc.When(`^the operator starts AlphOne$`, s.startAlphOne)
		sc.Then(`^the plugins that hide their screens are "([^"]*)"$`, s.hiddenAre)
		sc.Then(`^no plugin hides its screens$`, s.noneHidden)
		sc.Then(`^the server log holds no "([^"]*)" line$`, s.logHoldsNoLine)
		sc.Then(`^the server log holds one line with "([^"]*)"$`, s.logHoldsOneLine)
		sc.Then(`^the read is rejected as unauthenticated$`, s.rejectedUnsigned)
		sc.Then(`^AlphOne stops before it serves, naming the setting "([^"]*)"$`, s.stoppedNaming)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop()
		})
	}
}

// serveLogged starts AlphOne over the compiled plugins at a free address once, writing the scenario's log.
func (s *screensScenario) serveLogged() error {
	if s.stopServing != nil {
		return nil
	}
	address := testkit.FreeAddr(s.t)
	s.env["ALPHONE_ADDR"] = address
	serving, cancel := context.WithCancel(s.t.Context())
	s.stopServing, s.served = cancel, make(chan error, 1)
	getenv := testkit.Getenv(maps.Clone(s.env))
	go func() { s.served <- run(serving, getenv, &s.logged, registerPlugins) }()
	return s.awaitServer(address)
}

// readHidden keeps what the graph answers the member at email asking which plugins hide their screens.
func (s *screensScenario) readHidden(ctx context.Context, email string) error {
	if err := s.serveLogged(); err != nil {
		return err
	}
	return s.read(ctx, email, hiddenPluginsQuery, nil)
}

// readHiddenUnsigned keeps what the graph answers a caller with no credential asking which plugins hide their screens.
func (s *screensScenario) readHiddenUnsigned(ctx context.Context) error {
	if err := s.serveLogged(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"query": hiddenPluginsQuery})
	if err != nil {
		return err
	}
	address := "http://" + s.env["ALPHONE_ADDR"] + graphPath
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	s.reply = graphReply{}
	return json.NewDecoder(response.Body).Decode(&s.reply)
}

// startAlphOne runs AlphOne over the scenario's settings at a free address until it stops or the start budget ends.
func (s *screensScenario) startAlphOne() {
	s.env["ALPHONE_ADDR"] = testkit.FreeAddr(s.t)
	starting, cancel := context.WithTimeout(s.t.Context(), serveStartBudget)
	defer cancel()
	s.stopped = run(starting, testkit.Getenv(maps.Clone(s.env)), &s.logged, registerPlugins)
}

// hidden answers the ids the last read named as hiding their screens, an error for any graph error.
func (s *screensScenario) hidden() ([]string, error) {
	var ids []string
	if err := s.reply.decode(hiddenPluginsField, &ids); err != nil {
		return nil, err
	}
	if ids == nil {
		return nil, fmt.Errorf("the graph answered %s, want a list", s.replied())
	}
	return ids, nil
}

// hiddenAre fails unless the last read named exactly the plugins the text lists as hiding their screens, in order.
func (s *screensScenario) hiddenAre(want string) error {
	ids, err := s.hidden()
	if err != nil {
		return err
	}
	if listed := strings.Join(ids, ", "); listed != want {
		return fmt.Errorf("the plugins that hide their screens are %q, want %q", listed, want)
	}
	return nil
}

// noneHidden fails unless the last read named no plugin as hiding its screens.
func (s *screensScenario) noneHidden() error {
	ids, err := s.hidden()
	if err != nil {
		return err
	}
	if len(ids) != 0 {
		return fmt.Errorf("the plugins %q hide their screens, want none", ids)
	}
	return nil
}

// logHoldsNoLine fails unless the server log holds no line with text.
func (s *screensScenario) logHoldsNoLine(text string) error {
	if logged := s.logged.String(); strings.Contains(logged, text) {
		return fmt.Errorf("the server log holds %q, want no line with %q", logged, text)
	}
	return nil
}

// logHoldsOneLine fails unless the server log holds exactly one line with text.
func (s *screensScenario) logHoldsOneLine(text string) error {
	if logged := s.logged.String(); strings.Count(logged, text) != 1 {
		return fmt.Errorf("the server log holds %q, want one line with %q", logged, text)
	}
	return nil
}

// rejectedUnsigned fails unless the last read was rejected as coming from a caller who is not signed in.
func (s *screensScenario) rejectedUnsigned() error {
	unsigned := func(fault graphFault) bool { return fault.Extensions["code"] == unauthenticatedCode }
	if _, answered := s.reply.Data[hiddenPluginsField]; answered || !s.reply.rejects(unsigned) {
		return fmt.Errorf("the graph answered %s, want the read rejected as %s", s.replied(), unauthenticatedCode)
	}
	return nil
}

// stoppedNaming fails unless AlphOne stopped with an error naming the setting called key.
func (s *screensScenario) stoppedNaming(key string) error {
	if s.stopped == nil || !strings.Contains(s.stopped.Error(), key) {
		return fmt.Errorf("AlphOne stopped with %v, want it stopped before it served, naming %s", s.stopped, key)
	}
	return nil
}

func TestPluginScreens(t *testing.T) {
	runFeature(t, pluginScreensFeature, initializePluginScreens(t))
}
