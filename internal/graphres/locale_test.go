// SPDX-License-Identifier: Elastic-2.0

package graphres_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/graphres"
	"github.com/gopherium/alphone/internal/postgres"
)

// seedLocaleUser stores one account row and returns its id.
func seedLocaleUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at)
		VALUES ($1, $2, 'Maria Perez', 'hash', false, now())`, id, id.String()+"@example.com"); err != nil {
		t.Fatalf("storing the user: %v", err)
	}
	return id
}

func TestSetLocaleRoundTripsForASignedInCaller(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	caller := seedLocaleUser(t, pool)
	resolver := &graphres.Resolver{Settings: postgres.NewUserSettingStore(pool)}
	client := newGraphClient(t, resolver, caller)

	var set struct{ SetLocale string }
	if err := client.Post(`mutation { setLocale(locale: "es-ES") }`, &set); err != nil {
		t.Fatalf("setLocale error = %v, want nil", err)
	}
	if set.SetLocale != "es-ES" {
		t.Errorf("setLocale = %q, want the stored choice answered back", set.SetLocale)
	}

	var asked struct{ Locale string }
	if err := client.Post(`{ locale }`, &asked); err != nil {
		t.Fatalf("locale error = %v, want nil", err)
	}
	if asked.Locale != "es-ES" {
		t.Errorf("locale = %q, want the stored choice preferred over any header", asked.Locale)
	}
}

// failingSettings is a settings store whose reads and writes always fail.
type failingSettings struct{}

// UserSetting always reports the store unreachable.
func (failingSettings) UserSetting(context.Context, uuid.UUID, string) (string, error) {
	return "", errSettingsDown
}

// SetUserSetting always reports the store unreachable.
func (failingSettings) SetUserSetting(context.Context, uuid.UUID, string, string) error {
	return errSettingsDown
}

// errSettingsDown is the failure the failing settings store reports.
var errSettingsDown = errors.New("settings store down")

func TestLocaleReportsAFailingSettingsStore(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, &graphres.Resolver{Settings: failingSettings{}}, uuid.Must(uuid.NewV7()))

	if err := client.Post(`{ locale }`, &struct{ Locale string }{}); err == nil {
		t.Error("locale error = nil, want the store failure reported rather than a guessed locale")
	}
}

func TestSetLocaleReportsAFailingSettingsStore(t *testing.T) {
	t.Parallel()

	client := newGraphClient(t, &graphres.Resolver{Settings: failingSettings{}}, uuid.Must(uuid.NewV7()))

	if err := client.Post(`mutation { setLocale(locale: "es-ES") }`, &struct{ SetLocale string }{}); err == nil {
		t.Error("setLocale error = nil, want the store failure reported rather than a silent drop")
	}
}

func TestLocaleReadsTheHeaderTheTransportCarries(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/api/graphql", nil)
	request.Header.Set("Accept-Language", "es")
	client := newDecoratedGraphClient(t, &graphres.Resolver{}, func(ctx context.Context) context.Context {
		return graphres.WithHTTP(ctx, httptest.NewRecorder(), request)
	})

	var asked struct{ Locale string }
	if err := client.Post(`{ locale }`, &asked); err != nil {
		t.Fatalf("locale error = %v, want nil", err)
	}
	if asked.Locale != "es-ES" {
		t.Errorf("locale = %q, want the header the transport carries matched", asked.Locale)
	}
}

func TestSetLocaleRefusesALocaleOutsideTheList(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	caller := seedLocaleUser(t, pool)
	resolver := &graphres.Resolver{Settings: postgres.NewUserSettingStore(pool)}
	client := newGraphClient(t, resolver, caller)

	response, err := client.RawPost(`mutation { setLocale(locale: "de-DE") }`)
	if err != nil {
		t.Fatalf("RawPost() error = %v, want nil", err)
	}
	if got := firstErrorCode(t, response.Errors); got != "VALIDATION" {
		t.Errorf("code = %q, want VALIDATION", got)
	}
}
