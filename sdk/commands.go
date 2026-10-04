// SPDX-License-Identifier: Elastic-2.0

package sdk

import "github.com/gopherium/framework/gonsole"

// Command is one command a plugin offers under its id.
type Command = gonsole.Command

// Call is what a plugin command receives when it runs.
type Call = gonsole.Call

// CommandProvider is implemented by plugins that offer commands under their id.
type CommandProvider = gonsole.Provider

// Env reads the settings under the program prefix.
type Env = gonsole.Env

// Misuse marks err as a misused command line, which exits with code 2.
var Misuse = gonsole.Misuse

// Bound narrows the values a Count or Duration setting accepts.
type Bound = gonsole.Bound

// AtMost refuses a value above highest.
var AtMost = gonsole.AtMost

// AllowZero accepts zero beside the values above it.
var AllowZero = gonsole.AllowZero

// Parse returns the setting read by parse, the fallback when it is empty, any error naming the setting.
func Parse[T any](e Env, name string, fallback T, parse func(string) (T, error)) (T, error) {
	return gonsole.Parse(e, name, fallback, parse)
}
