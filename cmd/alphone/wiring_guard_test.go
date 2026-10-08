// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/gopherium/framework/gonsole/testkit"
	"github.com/gopherium/framework/pluginkit"
	"github.com/gopherium/gouncer/authkit"
	authkitpg "github.com/gopherium/gouncer/authkit/postgres"

	"github.com/gopherium/alphone/internal/graphroot"
)

// unwiredResolverFields names the graph resolver fields the server leaves to their defaults on purpose.
var unwiredResolverFields = []string{"BatchWait"}

// unwiredServerFields names the server settings the server leaves to their defaults on purpose.
var unwiredServerFields = []string{"MaxStreamLifetime", "MaxStreamsPerUser"}

// zeroFields returns the exported fields of the struct v holds or points at that are zero, the skipped ones left out.
func zeroFields(v any, skipped []string) []string {
	held := reflect.Indirect(reflect.ValueOf(v))
	var zero []string
	for i := range held.NumField() {
		field := held.Type().Field(i)
		if field.IsExported() && !slices.Contains(skipped, field.Name) && held.Field(i).IsZero() {
			zero = append(zero, field.Name)
		}
	}
	return zero
}

// unnamedFields returns the fields of named that are zero or equal to the same field of fallback.
func unnamedFields(named, fallback any) []string {
	held, standing := reflect.ValueOf(named), reflect.ValueOf(fallback)
	var unnamed []string
	for i := range held.NumField() {
		if held.Field(i).IsZero() || reflect.DeepEqual(held.Field(i).Interface(), standing.Field(i).Interface()) {
			unnamed = append(unnamed, held.Type().Field(i).Name)
		}
	}
	return unnamed
}

// composedOverEverySetting returns the settings with every value filled and what compose built over them.
func composedOverEverySetting(t *testing.T) (runConfig, composed) {
	t.Helper()
	env := filledSettings()
	settings, err := loadRunConfig(testkit.Getenv(env))
	if err != nil {
		t.Fatalf("loadRunConfig() error = %v, want nil", err)
	}
	built, err := compose(t.Context(), composeConfigOf(t, env), registerPlugins)
	if err != nil || built.failed != nil {
		t.Fatalf("compose() = %v with %v failed, want every plugin registered", err, built.failed)
	}
	abandonAtEnd(t, built)
	return settings, built
}

func TestGraphRootCarriesEveryComposedValue(t *testing.T) {
	t.Parallel()

	settings, built := composedOverEverySetting(t)

	resolver := graphResolver(settings, built, authkit.New(authConfig(built.users)), discardLogger())

	if zero := zeroFields(resolver, unwiredResolverFields); len(zero) > 0 {
		t.Errorf("the graph resolver leaves %v zero, want each one wired or named among %v on purpose",
			zero, unwiredResolverFields)
	}
}

func TestGraphResolverCarriesEveryListSetting(t *testing.T) {
	t.Parallel()

	settings, built := composedOverEverySetting(t)
	fallback, err := loadRunConfig(testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}))
	if err != nil {
		t.Fatalf("loadRunConfig() over the address alone error = %v, want nil", err)
	}

	resolver := graphResolver(settings, built, authkit.New(authConfig(built.users)), discardLogger())

	if unnamed := unnamedFields(resolver.Paging, fallback.lists.paging); len(unnamed) > 0 {
		t.Errorf("Paging leaves %v zero or at the default, want the values the settings name", unnamed)
	}
	if unnamed := unnamedFields(resolver.Screens, fallback.lists.screens); len(unnamed) > 0 {
		t.Errorf("Screens leaves %v zero or at the default, want the values the settings name", unnamed)
	}
}

func TestServerConfigCarriesEveryComposedValue(t *testing.T) {
	t.Parallel()

	settings, built := composedOverEverySetting(t)
	auth := authkit.New(authConfig(built.users))
	graphRoot, err := graphroot.FromPlugins(graphResolver(settings, built, auth, discardLogger()), built.registered)
	if err != nil {
		t.Fatalf("FromPlugins() error = %v, want nil", err)
	}

	cfg := serverConfigOf(settings, built, pluginkit.NewHost(built.registered...), auth, graphRoot, discardLogger())

	if zero := zeroFields(cfg, unwiredServerFields); len(zero) > 0 {
		t.Errorf("the server settings leave %v zero, want each one wired or named among %v on purpose",
			zero, unwiredServerFields)
	}
}

func TestInviteConfigCarriesEverySetting(t *testing.T) {
	t.Parallel()

	named, err := loadRunConfig(testkit.Getenv(filledSettings()))
	if err != nil {
		t.Fatalf("loadRunConfig() over every setting error = %v, want nil", err)
	}
	fallback, err := loadRunConfig(testkit.Getenv(map[string]string{"ALPHONE_DATABASE_URL": unreachableDatabaseURL}))
	if err != nil {
		t.Fatalf("loadRunConfig() over the address alone error = %v, want nil", err)
	}

	held := reflect.ValueOf(inviteConfigOf(named, authkitpg.NewUserStore(nil)))
	standing := reflect.ValueOf(inviteConfigOf(fallback, authkitpg.NewUserStore(nil)))

	for i := range held.NumField() {
		if held.Field(i).IsZero() || held.Field(i).Interface() == standing.Field(i).Interface() {
			t.Errorf("InvitesConfig.%s = %v, want the value the settings name, neither zero nor the default",
				held.Type().Field(i).Name, held.Field(i))
		}
	}
}
