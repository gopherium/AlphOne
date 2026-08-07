// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWritesADeterministicMergedSchema(t *testing.T) {
	t.Chdir("../..")
	out := filepath.Join(t.TempDir(), "schema.graphql")

	if err := Run("gqlgen.yml", out); err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	first, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("reading the snapshot: %v", err)
	}

	if !strings.Contains(string(first), "type Query") {
		t.Error("snapshot misses type Query")
	}
	if !strings.Contains(string(first), "createTask") {
		t.Error("snapshot misses the createTask mutation")
	}

	if err := Run("gqlgen.yml", out); err != nil {
		t.Fatalf("second Run() error = %v, want nil", err)
	}
	second, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("re-reading the snapshot: %v", err)
	}
	if string(first) != string(second) {
		t.Error("two runs produced different bytes, the snapshot is not deterministic")
	}
}

func TestRunReportsAMissingConfig(t *testing.T) {
	t.Parallel()

	err := Run(filepath.Join(t.TempDir(), "absent.yml"), filepath.Join(t.TempDir(), "out.graphql"))

	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Errorf("Run(absent config) error = %v, want the load config failure", err)
	}
}

func TestRunReportsABrokenSchema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "broken.graphqls")
	if err := os.WriteFile(schemaPath, []byte("type {"), 0o644); err != nil {
		t.Fatalf("writing the broken schema: %v", err)
	}
	configPath := filepath.Join(dir, "gqlgen.yml")
	if err := os.WriteFile(configPath, []byte("schema:\n  - "+schemaPath+"\n"), 0o644); err != nil {
		t.Fatalf("writing the config: %v", err)
	}

	err := Run(configPath, filepath.Join(dir, "out.graphql"))

	if err == nil || !strings.Contains(err.Error(), "load schema") {
		t.Errorf("Run(broken schema) error = %v, want the load schema failure", err)
	}
}

func TestRunReportsAnUnwritableSnapshot(t *testing.T) {
	t.Chdir("../..")

	err := Run("gqlgen.yml", filepath.Join(t.TempDir(), "missing", "out.graphql"))

	if err == nil || !strings.Contains(err.Error(), "write snapshot") {
		t.Errorf("Run(unwritable snapshot) error = %v, want the write snapshot failure", err)
	}
}
