// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gopherium/framework/gonsole"
	"github.com/gopherium/framework/gonsole/testkit"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// addressReader runs one command over getenv, address being the database address it must use.
type addressReader func(t *testing.T, getenv func(string) string, address string) error

// commandFailure runs the command line over getenv and plugins and answers its error, nil when it succeeds.
func commandFailure(
	t *testing.T, getenv func(string) string, plugins func(sdk.Deps) ([]sdk.Plugin, error), args ...string,
) error {
	t.Helper()
	got := testkit.Run(t, programOver(role.NewRegistry(), getenv, plugins), typedPassword+"\n", args...)
	if got.Code == gonsole.ExitDone {
		return nil
	}
	return errors.New(strings.TrimSuffix(strings.TrimPrefix(got.Stderr, "alphone: "), "\n"))
}

// addressRequirers returns every command that refuses to run without a database address, by name.
func addressRequirers() map[string]addressReader {
	return map[string]addressReader{
		"account:create-admin": func(t *testing.T, getenv func(string) string, _ string) error {
			return commandFailure(t, getenv, registeringNothing,
				"account:create-admin", "-email", "admin@example.com", "-name", "Admin", "-role", "admin")
		},
		"account:grant-role": func(t *testing.T, getenv func(string) string, _ string) error {
			return commandFailure(t, getenv, registeringNothing,
				"account:grant-role", "-role", "member", "-yes", "-as", "maria.perez@example.com")
		},
		"token:list": func(t *testing.T, getenv func(string) string, _ string) error {
			return commandFailure(t, getenv, registeringNothing, "token:list", "-email", "maria.perez@example.com")
		},
		"seed": func(t *testing.T, getenv func(string) string, _ string) error {
			return seed(t.Context(), getenv, io.Discard)
		},
	}
}

// addressReaders returns every reader of the database address, the commands and the plugin roles, by name.
func addressReaders() map[string]addressReader {
	readers := addressRequirers()
	readers["the plugin roles"] = func(t *testing.T, getenv func(string) string, address string) error {
		return commandFailure(t, getenv, func(deps sdk.Deps) ([]sdk.Plugin, error) {
			if deps.DatabaseURL != address {
				return nil, fmt.Errorf("the plugins got the address %q, want %q", deps.DatabaseURL, address)
			}
			return nil, nil
		}, "account:grant-role", "-role", "member", "-as", "maria.perez@example.com")
	}
	return readers
}

func TestEveryCommandTakesAPaddedDatabaseAddress(t *testing.T) {
	t.Parallel()

	for name, read := range addressReaders() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			address := testDatabaseURL(t)
			createAccount(t, testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": address}),
				"maria.perez@example.com", role.Admin.String())
			getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": "  " + address + "  "})

			if err := read(t, getenv, address); err != nil {
				t.Errorf("%s over a padded address error = %v, want the address read trimmed", name, err)
			}
		})
	}
}

func TestEveryCommandRefusesABlankDatabaseAddressByName(t *testing.T) {
	t.Parallel()

	for name, read := range addressRequirers() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			getenv := testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": "   "})

			err := read(t, getenv, "")

			if err == nil || err.Error() != "ALPHONE_DATABASE_URL is required" {
				t.Errorf("%s over a blank address error = %v, want %q", name, err, "ALPHONE_DATABASE_URL is required")
			}
		})
	}
}
