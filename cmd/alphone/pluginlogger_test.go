// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// sweptLine is the info line the plugin logger tests write.
const sweptLine = "swept the old files"

// goneLine is the warning the plugin logger tests write.
const goneLine = "an old file was already gone"

// failedLine is the error the plugin logger tests write.
const failedLine = "could not delete an old file"

// loggerTakingPlugin keeps the logger its registration was handed.
type loggerTakingPlugin struct {
	inertPlugin
	logger *slog.Logger
}

// registerLoggerTaker returns a plugin under id keeping the logger deps carries, as a plugin's Register does.
func registerLoggerTaker(id string, deps sdk.Deps) *loggerTakingPlugin {
	return &loggerTakingPlugin{inertPlugin: inertPlugin{id: id}, logger: deps.Logger}
}

// reportFailures writes one info line, one warning and one error through logger, failing the test when it is nil.
func reportFailures(t *testing.T, logger *slog.Logger) {
	t.Helper()
	if logger == nil {
		t.Fatal("the plugin received no logger, want the host's")
	}
	logger.Info(sweptLine)
	logger.Warn(goneLine)
	logger.Error(failedLine)
}

func TestComposeHandsEachPluginTheHostLogger(t *testing.T) {
	t.Parallel()

	cfg := composeConfigOf(t, filledSettings())
	var taking []*loggerTakingPlugin

	built, err := compose(t.Context(), cfg, func(deps sdk.Deps) ([]sdk.Plugin, error) {
		taking = []*loggerTakingPlugin{registerLoggerTaker("notes", deps), registerLoggerTaker("mail", deps)}
		return []sdk.Plugin{taking[0], taking[1]}, nil
	})

	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	for _, plugin := range taking {
		if plugin.logger != cfg.logger {
			t.Errorf("the %s plugin received the logger %p, want the host's %p", plugin.id, plugin.logger, cfg.logger)
		}
	}
}

func TestRunHandsPluginsTheServerLog(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	var handed sdk.Deps
	getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL})

	err := run(t.Context(), getenv, &stderr, capturingPlugins(&handed))

	if !errors.Is(err, errCaptured) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errCaptured)
	}
	reportFailures(t, handed.Logger)
	for _, want := range []string{
		"level=INFO msg=" + strconv.Quote(sweptLine),
		"level=WARN msg=" + strconv.Quote(goneLine),
		"level=ERROR msg=" + strconv.Quote(failedLine),
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want the plugin's line %s in the server log", stderr.String(), want)
		}
	}
}

func TestLoadPluginsShowsOnlyThePluginsWarningsAndErrors(t *testing.T) {
	t.Parallel()

	var stderr strings.Builder
	var logger *slog.Logger

	loaded, err := loadPlugins(role.NewRegistry(), func(deps sdk.Deps) ([]sdk.Plugin, error) {
		logger = deps.Logger
		return nil, nil
	})(t.Context(), describingCall(nil, &stderr))

	if err != nil {
		t.Fatalf("loadPlugins() error = %v, want nil", err)
	}
	releaseAtEnd(t, loaded)
	reportFailures(t, logger)
	if strings.Contains(stderr.String(), sweptLine) {
		t.Errorf("stderr = %q, want the plugin's info line kept off a command's stderr", stderr.String())
	}
	for _, want := range []string{
		"level=WARN msg=" + strconv.Quote(goneLine),
		"level=ERROR msg=" + strconv.Quote(failedLine),
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want the plugin's line %s on a command's stderr", stderr.String(), want)
		}
	}
}
