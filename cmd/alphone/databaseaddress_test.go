// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/gopherium/alphone/internal/role"
	"github.com/gopherium/alphone/sdk"
)

// addressReader runs one command over getenv, address being the database address it must use.
type addressReader func(t *testing.T, getenv func(string) string, address string) error

// addressRequirers returns every command that refuses to run without a database address, by name.
func addressRequirers() map[string]addressReader {
	return map[string]addressReader{
		"createadmin": func(t *testing.T, getenv func(string) string, _ string) error {
			return createAdmin(t.Context(), getenv,
				[]string{"-email", "admin@example.com", "-name", "Admin", "-role", "admin"},
				strings.NewReader("correct horse battery\n"), io.Discard)
		},
		"grantrole": func(t *testing.T, getenv func(string) string, _ string) error {
			return grantRole(t.Context(), getenv, []string{"-role", "member"}, io.Discard)
		},
		"token": func(t *testing.T, getenv func(string) string, _ string) error {
			return token(t.Context(), getenv, []string{"list", "-email", "maria.perez@example.com"}, io.Discard)
		},
		"seed": func(t *testing.T, getenv func(string) string, _ string) error {
			return seed(t.Context(), getenv, io.Discard)
		},
	}
}

// addressReaders returns every reader of the database address, the commands and the plugin roles, by name.
func addressReaders() map[string]addressReader {
	readers := addressRequirers()
	readers["the plugin roles"] = func(_ *testing.T, getenv func(string) string, address string) error {
		return declarePluginRoles(role.NewRegistry(), getenv, func(deps sdk.Deps) ([]sdk.Plugin, error) {
			if deps.DatabaseURL != address {
				return nil, fmt.Errorf("the plugins got the address %q, want %q", deps.DatabaseURL, address)
			}
			return nil, nil
		})
	}
	return readers
}

func TestEveryCommandTakesAPaddedDatabaseAddress(t *testing.T) {
	t.Parallel()

	for name, read := range addressReaders() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			address := testDatabaseURL(t)
			storeRoleless(t, address, "maria.perez@example.com")
			getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": "  " + address + "  "})

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

			getenv := testGetenv(map[string]string{"ALPHONE_DATABASE_URL": "   "})

			err := read(t, getenv, "")

			if err == nil || err.Error() != "ALPHONE_DATABASE_URL is required" {
				t.Errorf("%s over a blank address error = %v, want %q", name, err, "ALPHONE_DATABASE_URL is required")
			}
		})
	}
}

func TestRoleWritingSubcommandsPrintTheirHelpWithoutADatabaseAddress(t *testing.T) {
	t.Setenv("ALPHONE_DATABASE_URL", "")
	noPlugins := func(sdk.Deps) ([]sdk.Plugin, error) { return nil, nil }

	for _, name := range []string{"createadmin", "grantrole"} {
		if err := dispatch(t.Context(), []string{name, "-h"}, noPlugins); err != nil {
			t.Errorf("dispatch(%s -h) error = %v, want the help printed before the address is read", name, err)
		}
	}
}
