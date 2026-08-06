// SPDX-License-Identifier: Elastic-2.0

// Command schemagen writes the merged GraphQL schema snapshot.
package main

import (
	"fmt"
	"os"
)

// main writes the schema snapshot next to the gqlgen config.
func main() {
	if err := Run("gqlgen.yml", "graph/schema.graphql"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
