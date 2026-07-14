// SPDX-License-Identifier: Elastic-2.0

// Package alphone exposes application-wide metadata.
package alphone

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var version string

// Version returns the application version.
func Version() string {
	return strings.TrimSpace(version)
}
