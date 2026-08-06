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
