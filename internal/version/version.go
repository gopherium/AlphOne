// SPDX-License-Identifier: Elastic-2.0

// Package version exposes application-wide metadata.
package version

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var raw string

// Version returns the application version.
func Version() string {
	return strings.TrimSpace(raw)
}
