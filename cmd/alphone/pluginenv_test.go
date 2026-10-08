// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"io"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// mediaCapSetting is the environment of a plugin setting the settings reader must find under the program prefix.
var mediaCapSetting = map[string]string{"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": "1024"}

// assertReadsThePluginSetting fails the test unless the handed reader finds the plugin setting under the prefix.
func assertReadsThePluginSetting(t *testing.T, handed sdk.Deps) {
	t.Helper()
	if held := handed.Env.Within("WHATSAPP_").Value("MEDIA_MAX_BYTES"); held != "1024" {
		t.Errorf("Env reads WHATSAPP_MEDIA_MAX_BYTES as %q, want 1024 under the program prefix", held)
	}
}

func TestRunHandsThePluginsTheirSettingsReader(t *testing.T) {
	t.Parallel()

	var handed sdk.Deps
	err := run(t.Context(), testkit.Getenv(map[string]string{
		"ALPHONE_DATABASE_URL":             testDatabaseURL(t),
		"ALPHONE_WHATSAPP_MEDIA_MAX_BYTES": mediaCapSetting["ALPHONE_WHATSAPP_MEDIA_MAX_BYTES"],
	}), io.Discard, capturingPlugins(&handed))

	if !errors.Is(err, errCaptured) {
		t.Fatalf("run() error = %v, want %v in its chain", err, errCaptured)
	}
	assertReadsThePluginSetting(t, handed)
}

func TestLoadPluginsHandsThePluginsTheirSettingsReader(t *testing.T) {
	t.Parallel()

	var handed sdk.Deps
	loaded, err := loadPlugins(role.NewRegistry(), capturingPlugins(&handed))(t.Context(),
		describingCall(mediaCapSetting, io.Discard))

	if err != nil || !errors.Is(loaded.Failed, errCaptured) {
		t.Fatalf("loadPlugins() = %v with %v failed, want %v failed", err, loaded.Failed, errCaptured)
	}
	releaseAtEnd(t, loaded)
	assertReadsThePluginSetting(t, handed)
}
