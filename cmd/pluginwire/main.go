// SPDX-License-Identifier: Elastic-2.0

// Command pluginwire regenerates the plugin wiring files from the
// plugin.json manifest of every directory under plugins/.
package main

import (
	"fmt"
	"os"
)

// main regenerates the plugin wiring files.
func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
