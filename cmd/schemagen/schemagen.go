// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"bytes"
	"fmt"
	"os"

	"github.com/99designs/gqlgen/codegen/config"
	"github.com/vektah/gqlparser/v2/formatter"
)

// Run writes the merged formatted schema of the gqlgen config to outPath.
func Run(configPath, outPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("schemagen: load config: %w", err)
	}
	if err := cfg.LoadSchema(); err != nil {
		return fmt.Errorf("schemagen: load schema: %w", err)
	}
	var buf bytes.Buffer
	formatter.NewFormatter(&buf).FormatSchema(cfg.Schema)
	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("schemagen: write snapshot: %w", err)
	}
	return nil
}
