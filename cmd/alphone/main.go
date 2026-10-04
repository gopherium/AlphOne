// SPDX-License-Identifier: Elastic-2.0

// Command alphone runs the AlphOne CRM server and the commands an operator runs beside it.
package main

import (
	"os"

	"github.com/joho/godotenv"

	"github.com/gopherium/framework/gonsole"
)

// main runs the alphone command line.
func main() {
	_ = godotenv.Load()
	os.Exit(gonsole.Main(program(os.Getenv, registerPlugins)))
}
