// SPDX-License-Identifier: Elastic-2.0

// Command doclint fails when any Go function under the module lacks a
// doc comment, enforcing full documentation coverage.
package main

import (
	"fmt"
	"os"
)

// main runs the documentation check over the current directory tree.
func main() {
	if err := run(".", os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
