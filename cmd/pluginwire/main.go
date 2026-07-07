// SPDX-License-Identifier: AGPL-3.0-or-later

// Command pluginwire regenerates the plugin wiring files from the
// plugin.json manifest of every directory under plugins/.
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run("."); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
