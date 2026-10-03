// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/pluginkit/wire"
)

func TestConfigRefusesAPluginIDTheCommandLineKeeps(t *testing.T) {
	t.Parallel()

	for _, id := range append(gonsole.BaseCommands(), "account", "token") {
		t.Run(id, func(t *testing.T) {
			t.Parallel()

			root := t.TempDir()
			writeTree(t, root)
			writePluginIn(t, root, "plugins", id, fmt.Sprintf(
				`{"id": %q, "name": "Taken", "backend": "github.com/gopherium/alphone/plugins/%s"}`, id, id))

			err := wire.Run(root, config)

			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("id %q is reserved", id)) {
				t.Errorf("wire.Run() error = %v, want the id %q refused as reserved", err, id)
			}
		})
	}
}

func TestRepositoryWiringIsUpToDate(t *testing.T) {
	t.Parallel()

	repo := filepath.Join("..", "..")
	tmp := t.TempDir()
	for _, pluginRoot := range roots {
		if err := os.MkdirAll(filepath.Join(tmp, pluginRoot), 0o755); err != nil {
			t.Fatalf("creating %s: %v", pluginRoot, err)
		}
		entries, err := os.ReadDir(filepath.Join(repo, pluginRoot))
		if err != nil {
			t.Fatalf("reading the %s directory: %v", pluginRoot, err)
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifest, err := os.ReadFile(filepath.Join(repo, pluginRoot, entry.Name(), "plugin.json"))
			if err != nil {
				t.Fatalf("reading manifest: %v", err)
			}
			writePluginIn(t, tmp, pluginRoot, entry.Name(), string(manifest))
		}
	}
	for _, dir := range []string{"cmd/alphone", "frontend/src/plugins", "internal/graphroot"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}

	if err := wire.Run(tmp, config); err != nil {
		t.Fatalf("wire.Run() error = %v, want nil", err)
	}

	for _, generated := range []string{config.GoWiringPath, config.TSWiringPath, config.GoRegistryPath} {
		fresh, err := os.ReadFile(filepath.Join(tmp, generated))
		if err != nil {
			t.Fatalf("reading fresh %s: %v", generated, err)
		}
		committed, err := os.ReadFile(filepath.Join(repo, generated))
		if err != nil {
			t.Fatalf("reading committed %s: %v", generated, err)
		}
		if string(fresh) != string(committed) {
			t.Errorf("%s is stale, run make generate", generated)
		}
	}
}
