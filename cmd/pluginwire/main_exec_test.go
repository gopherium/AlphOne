// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writePluginIn writes a plugin manifest under the named plugin root.
func writePluginIn(t *testing.T, root, pluginRoot, dir, manifestJSON string) {
	t.Helper()
	pluginDir := filepath.Join(root, pluginRoot, dir)
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("creating %s: %v", pluginDir, err)
	}
	if manifestJSON == "" {
		return
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifestJSON), 0o644); err != nil {
		t.Fatalf("writing manifest: %v", err)
	}
}

// writeTree creates every directory the generators write into, plus the core schema.
func writeTree(t *testing.T, root string) {
	t.Helper()
	dirs := []string{"cmd/alphone", "frontend/src/plugins", "internal/graphroot", "enterprise", "graph/schema"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}
	coreSDL := "type Query {\n\tversion: String!\n}\n"
	if err := os.WriteFile(filepath.Join(root, "graph", "schema", "core.graphqls"), []byte(coreSDL), 0o644); err != nil {
		t.Fatalf("writing core schema: %v", err)
	}
}

func coverBinary(t *testing.T) (string, []string) {
	t.Helper()
	bindir := os.Getenv("ALPHONE_COVER_BINDIR")
	gocoverdir := os.Getenv("ALPHONE_COVER_GOCOVERDIR")
	if bindir == "" || gocoverdir == "" {
		t.Skip("skipping binary test: run via make cover")
	}
	var env []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "ALPHONE_") && !strings.HasPrefix(entry, "GOCOVERDIR=") {
			env = append(env, entry)
		}
	}
	return filepath.Join(bindir, "pluginwire"), append(env, "GOCOVERDIR="+gocoverdir)
}

func TestMainBinaryGeneratesWiring(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	root := t.TempDir()
	writeTree(t, root)
	writePluginIn(t, root, "plugins", "demo", `{
		"id": "demo",
		"name": "Demo",
		"backend": "github.com/gopherium/alphone/plugins/demo"
	}`)
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("pluginwire on a valid tree: %v, stderr: %s", err, stderr.String())
	}

	generatedFiles := []string{
		"cmd/alphone/plugins_gen.go",
		"frontend/src/plugins/index.ts",
		"internal/graphroot/graphroot_gen.go",
	}
	for _, generated := range generatedFiles {
		if _, err := os.Stat(filepath.Join(root, generated)); err != nil {
			t.Errorf("expected generated file %s: %v", generated, err)
		}
	}
}

func TestMainBinaryWiresTheEnterpriseRoot(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	root := t.TempDir()
	writeTree(t, root)
	writePluginIn(t, root, "plugins", "demo", `{
		"id": "demo",
		"name": "Demo",
		"backend": "github.com/gopherium/alphone/plugins/demo"
	}`)
	writePluginIn(t, root, "enterprise", "tenancy", `{
		"id": "tenancy",
		"name": "Tenancy",
		"backend": "github.com/gopherium/alphone/enterprise/tenancy"
	}`)
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("pluginwire on a tree with both roots: %v, stderr: %s", err, stderr.String())
	}

	wiring, err := os.ReadFile(filepath.Join(root, "cmd", "alphone", "plugins_gen.go"))
	if err != nil {
		t.Fatalf("reading the generated wiring: %v", err)
	}
	if !strings.Contains(string(wiring), "alphone/enterprise/tenancy") {
		t.Errorf("plugins_gen.go = %q, want the enterprise plugin wired", wiring)
	}
}

func TestMainBinaryEmptyEnterpriseRootReproducesTheCommittedBytes(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	root := t.TempDir()
	writeTree(t, root)
	writePluginIn(t, root, "plugins", "demo", `{
		"id": "demo",
		"name": "Demo",
		"backend": "github.com/gopherium/alphone/plugins/demo"
	}`)
	readme := filepath.Join(root, "enterprise", "README.md")
	if err := os.WriteFile(readme, []byte("enterprise plugins land here"), 0o644); err != nil {
		t.Fatalf("writing the enterprise README: %v", err)
	}
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("pluginwire with an empty enterprise root: %v, stderr: %s", err, stderr.String())
	}

	for _, generated := range []string{
		"cmd/alphone/plugins_gen.go",
		"frontend/src/plugins/index.ts",
		"internal/graphroot/graphroot_gen.go",
	} {
		content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(generated)))
		if err != nil {
			t.Fatalf("reading %s: %v", generated, err)
		}
		if strings.Contains(string(content), "enterprise") {
			t.Errorf("%s names the enterprise root while it holds no plugins", generated)
		}
	}
}

func TestMainBinaryFailsWithoutCoreSchemas(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	root := t.TempDir()
	writePluginIn(t, root, "plugins", "demo", `{
		"id": "demo",
		"name": "Demo",
		"backend": "github.com/gopherium/alphone/plugins/demo"
	}`)
	for _, dir := range []string{"cmd/alphone", "frontend/src/plugins", "enterprise", "internal/graphroot"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("creating %s: %v", dir, err)
		}
	}
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("pluginwire without core schemas: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "graphwire") {
		t.Errorf("stderr = %q, want it to report the graphwire failure", stderr.String())
	}
}

func TestMainBinaryFailsWithoutPluginsDirectory(t *testing.T) {
	t.Parallel()

	binary, env := coverBinary(t)
	var stderr bytes.Buffer
	cmd := exec.Command(binary)
	cmd.Dir = t.TempDir()
	cmd.Env = env
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("pluginwire without a plugins directory: %v, want exit code 1", err)
	}
	if !strings.Contains(stderr.String(), "reading plugins directory") {
		t.Errorf("stderr = %q, want it to report the missing plugins directory", stderr.String())
	}
}
