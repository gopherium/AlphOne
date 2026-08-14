// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gopherium/pluginkit/wire"
)

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
	for _, dir := range []string{"cmd/alphone", "frontend/src/plugins"} {
		if err := os.MkdirAll(filepath.Join(tmp, dir), 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}

	if err := wire.Run(tmp, config); err != nil {
		t.Fatalf("wire.Run() error = %v, want nil", err)
	}

	for _, generated := range []string{config.GoWiringPath, config.TSWiringPath} {
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
