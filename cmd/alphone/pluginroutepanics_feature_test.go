// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cucumber/godog"

	"github.com/gopherium/framework/gonsole/testkit"
)

// pluginRoutePanicsFeature is the feature a panic in a plugin route answers to.
const pluginRoutePanicsFeature = "../../test/features/features/plugin-route-panics.feature"

// pluginRoutesPath is where AlphOne serves the routes of every plugin, before the plugin's id.
const pluginRoutesPath = "/api/plugins/"

// routePanicLine is the message the server logs for each panic of a plugin route.
const routePanicLine = "plugin route panicked"

// panicWorkspace is the workspace the member holding a token stands in.
const panicWorkspace = "Workspace"

// panicHolder is the address of the member holding a token.
const panicHolder = "member@example.com"

// cutBudget bounds how long a request for a cut answer waits before it counts as a timeout.
const cutBudget = 10 * time.Second

// routeBoom is a route of the probe that panics with text, after it began a 200 answer when after is set.
type routeBoom struct {
	text  string
	after bool
}

// serve panics with the boom's text, first sending part of a 200 answer when after is set.
func (b routeBoom) serve(w http.ResponseWriter) {
	if b.after {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "partial")
		_ = http.NewResponseController(w).Flush()
	}
	panic(b.text)
}

// probePlugin is a plugin whose routes panic on each path a scenario set and answer 200 on the rest.
type probePlugin struct {
	inertPlugin
	public []string
	booms  map[string]routeBoom
}

// PublicPaths opens the routes the scenario named public to callers without a token.
func (p *probePlugin) PublicPaths() []string {
	return p.public
}

// Routes returns the handler panicking on each path the scenario set and answering 200 on the rest.
func (p *probePlugin) Routes() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		boom, panics := p.booms[r.URL.Path]
		if !panics {
			w.WriteHeader(http.StatusOK)
			return
		}
		boom.serve(w)
	})
}

// panicScenario is what one plugin route panic scenario keeps between its steps, beside what the read steps keep.
type panicScenario struct {
	*readScenario
	plugin *probePlugin
	logged lockedLog
}

// initializePluginRoutePanics returns the binding of the plugin route panic steps, t holding their databases.
func initializePluginRoutePanics(t *testing.T) func(*godog.ScenarioContext) {
	return func(sc *godog.ScenarioContext) {
		s := &panicScenario{readScenario: newReadScenario(newEmailScenario(t))}
		sc.Given(`^the settings point at a migrated database$`, s.migrateTheDatabase)
		sc.Given(`^AlphOne runs the plugin "([^"]*)", whose public route "([^"]*)" answers 200$`, s.runProbe)
		sc.Given(`^the public route "([^"]*)" of "([^"]*)" (answers 200, then )?panics with "([^"]*)"$`,
			s.panicsInPublic)
		sc.Given(`^the route "([^"]*)" of "([^"]*)" needs a token and panics with "([^"]*)"$`, s.panicsBehindAToken)
		sc.Given(`^a member of a workspace holds an API token$`, s.holdAMembersToken)
		sc.When(`^AlphOne serves$`, s.serveProbe)
		sc.Then(`^the route "([^"]*)" of "([^"]*)" answers (\d+) with the code "([^"]*)", kept out of every cache$`,
			s.answersPublicCode)
		sc.Then(`^the route "([^"]*)" of "([^"]*)" answers that token (\d+) with the code "([^"]*)", `+
			`kept out of every cache$`, s.answersTokenCode)
		sc.Then(`^the route "([^"]*)" of "([^"]*)" answers (\d+)$`, s.answersStatus)
		sc.Then(`^the answer of the route "([^"]*)" of "([^"]*)" is cut$`, s.answerIsCut)
		sc.Then(`^the server log holds once that (\S+) "([^"]*)" of "([^"]*)" panicked with "([^"]*)", `+
			`(answered|not answered)$`, s.loggedOnce)
		sc.Then(`^the server log holds no "([^"]*)" line$`, s.logHoldsNoLine)
		sc.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
			return ctx, s.stop()
		})
	}
}

// runProbe has AlphOne run the plugin id, whose public route at path answers 200.
func (s *panicScenario) runProbe(id, path string) {
	s.plugin = &probePlugin{inertPlugin: inertPlugin{id: id}, public: []string{path}, booms: map[string]routeBoom{}}
}

// probeOf returns the plugin AlphOne runs as id, an error when it runs no plugin of that id.
func (s *panicScenario) probeOf(id string) (*probePlugin, error) {
	if s.plugin == nil || s.plugin.id != id {
		return nil, fmt.Errorf("AlphOne runs no plugin %q", id)
	}
	return s.plugin, nil
}

// panicsInPublic makes the public route at path of the plugin id panic with text, after it answered 200 when set.
func (s *panicScenario) panicsInPublic(path, id, after, text string) error {
	probe, err := s.probeOf(id)
	if err != nil {
		return err
	}
	probe.public = append(probe.public, path)
	probe.booms[path] = routeBoom{text: text, after: after != ""}
	return nil
}

// panicsBehindAToken makes the route at path of the plugin id, which needs a credential, panic with text.
func (s *panicScenario) panicsBehindAToken(path, id, text string) error {
	probe, err := s.probeOf(id)
	if err != nil {
		return err
	}
	probe.booms[path] = routeBoom{text: text}
	return nil
}

// holdAMembersToken stands a new member in a new workspace and mints that member a token holding every scope.
func (s *panicScenario) holdAMembersToken(ctx context.Context) error {
	if err := s.createWorkspace(ctx, panicWorkspace); err != nil {
		return err
	}
	if err := s.holdMember(panicHolder); err != nil {
		return err
	}
	if err := s.placeAccount(ctx, panicHolder, panicWorkspace); err != nil {
		return err
	}
	return s.holdToken(panicHolder, "")
}

// serveProbe starts AlphOne serving the compiled plugins and the probe at a free address, writing the scenario's log.
func (s *panicScenario) serveProbe() error {
	address := testkit.FreeAddr(s.t)
	s.env["ALPHONE_ADDR"] = address
	serving, cancel := context.WithCancel(s.t.Context())
	s.stopServing, s.served = cancel, make(chan error, 1)
	getenv := testkit.Getenv(maps.Clone(s.env))
	plugins := besideTheCompiledPlugins(s.plugin)
	go func() { s.served <- run(serving, getenv, &s.logged, plugins) }()
	return s.awaitServer(address)
}

// routeAddress returns where AlphOne serves the route at path of the plugin id.
func (s *panicScenario) routeAddress(id, path string) string {
	return "http://" + s.env["ALPHONE_ADDR"] + pluginRoutesPath + id + path
}

// get answers what client gets for a GET of the route at path of the plugin id, sent with no credential.
func (s *panicScenario) get(ctx context.Context, client *http.Client, id, path string) (httpAnswer, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.routeAddress(id, path), nil)
	if err != nil {
		return httpAnswer{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return httpAnswer{}, err
	}
	defer func() { _ = response.Body.Close() }()
	read, err := io.ReadAll(response.Body)
	return httpAnswer{status: response.StatusCode, header: response.Header, body: read}, err
}

// answersPublicCode fails unless the route at path of the plugin id answers status with the code to any caller.
func (s *panicScenario) answersPublicCode(ctx context.Context, path, id string, status int, code string) error {
	got, err := s.get(ctx, http.DefaultClient, id, path)
	return answeredCode(got, err, id, status, code)
}

// answersTokenCode fails unless the route at path of the plugin id answers the member's token status with the code.
func (s *panicScenario) answersTokenCode(ctx context.Context, path, id string, status int, code string) error {
	got, err := s.call(ctx, panicHolder, http.MethodGet, pluginRoutesPath+id+path, nil)
	return answeredCode(got, err, id, status, code)
}

// answeredCode fails unless got holds status with the code, never names the plugin id and is kept out of every cache.
func answeredCode(got httpAnswer, err error, id string, status int, code string) error {
	if err != nil {
		return fmt.Errorf("the route of %s got no whole answer: %w", id, err)
	}
	var answered struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(got.body, &answered); err != nil || got.status != status || answered.Code != code {
		return fmt.Errorf("the route of %s answered %d %s, want %d with the code %s", id, got.status, got.body,
			status, code)
	}
	if bytes.Contains(got.body, []byte(id)) {
		return fmt.Errorf("the answer %s names the plugin %s", got.body, id)
	}
	if control := got.header.Get("Cache-Control"); !strings.Contains(control, "no-store") {
		return fmt.Errorf("the answer carries Cache-Control %q, want no-store", control)
	}
	return nil
}

// answersStatus fails unless the route at path of the plugin id answers status to any caller.
func (s *panicScenario) answersStatus(ctx context.Context, path, id string, status int) error {
	got, err := s.get(ctx, http.DefaultClient, id, path)
	if err != nil || got.status != status {
		return fmt.Errorf("the route %s of %s answered %d (%v), want %d", path, id, got.status, err, status)
	}
	return nil
}

// answerIsCut fails unless a GET of the route, on a connection of its own, ends cut short and not for a timeout.
func (s *panicScenario) answerIsCut(ctx context.Context, path, id string) error {
	transport := &http.Transport{DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	got, err := s.get(ctx, &http.Client{Transport: transport, Timeout: cutBudget}, id, path)
	var netErr net.Error
	if err == nil || errors.As(err, &netErr) && netErr.Timeout() {
		return fmt.Errorf("the route %s of %s answered %d %q (%v), want the answer cut", path, id, got.status,
			got.body, err)
	}
	return nil
}

// loggedOnce fails unless the server logged one route panic, of the plugin id for the request, answered or not.
func (s *panicScenario) loggedOnce(method, path, id, text, how string) error {
	logged := s.logged.String()
	if count := strings.Count(logged, fmt.Sprintf("msg=%q", routePanicLine)); count != 1 {
		return fmt.Errorf("the server logged %d route panics, want 1:\n%s", count, logged)
	}
	want := fmt.Sprintf("msg=%q plugin=%s method=%s path=%s answered=%t error=%q stack=\"goroutine ",
		routePanicLine, id, method, path, how == "answered", "pluginkit: "+id+" route panicked: "+text)
	if !strings.Contains(logged, want) {
		return fmt.Errorf("the server logged %q, want the line %s", logged, want)
	}
	return nil
}

// logHoldsNoLine fails unless the server log holds no line with text.
func (s *panicScenario) logHoldsNoLine(text string) error {
	if logged := s.logged.String(); strings.Contains(logged, text) {
		return fmt.Errorf("the server log holds %q, want no line with %q", logged, text)
	}
	return nil
}

func TestPluginRoutePanics(t *testing.T) {
	runFeature(t, pluginRoutePanicsFeature, initializePluginRoutePanics(t))
}
