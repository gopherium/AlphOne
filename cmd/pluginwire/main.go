// SPDX-License-Identifier: Elastic-2.0

// Command pluginwire regenerates the plugin wiring files from the
// plugin.json manifest of every directory under plugins/.
package main

import (
	"fmt"
	"os"

	"github.com/gopherium/pluginkit/graphwire"
	"github.com/gopherium/pluginkit/wire"
)

// config parameterizes the shared wiring generator for AlphOne.
var config = wire.Config{
	SDKImport:    "github.com/gopherium/alphone/sdk",
	FrontendSDK:  "@alphone/frontend-sdk",
	GoWiringPath: "cmd/alphone/plugins_gen.go",
	TSWiringPath: "frontend/src/plugins/index.ts",
	License:      "Elastic-2.0",
	TSLicense:    "AGPL-3.0-or-later",
}

// graphConfig parameterizes the graph resolver root generator for AlphOne.
var graphConfig = graphwire.Config{
	ExecImport:      "github.com/gopherium/alphone/graph",
	CoreImport:      "github.com/gopherium/alphone/internal/graphres",
	CoreSchemaGlobs: []string{"graph/schema/*.graphqls"},
	WiringPath:      "internal/graphroot/graphroot_gen.go",
	License:         "Elastic-2.0",
	Package:         "graphroot",
	SDKImport:       "github.com/gopherium/alphone/sdk",
}

// main regenerates the plugin wiring files.
func main() {
	if err := wire.Run(".", config); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := graphwire.Run(".", graphConfig); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
