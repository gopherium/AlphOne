// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/postgres"
)

// seedSettingsUser stores one account row for a settings test to hang from.
func seedSettingsUser(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id := uuid.Must(uuid.NewV7())
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO auth.users (id, email, name, password_hash, disabled, created_at)
		VALUES ($1, $2, 'Maria Perez', 'hash', false, now())`, id, id.String()+"@example.com"); err != nil {
		t.Fatalf("storing the user: %v", err)
	}
	return id
}

func TestUserSettingRoundTrips(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserSettingStore(pool)
	user := seedSettingsUser(t, pool)

	if err := store.SetUserSetting(t.Context(), user, "locale.default", "es-ES"); err != nil {
		t.Fatalf("SetUserSetting() error = %v, want nil", err)
	}

	got, err := store.UserSetting(t.Context(), user, "locale.default")
	if err != nil {
		t.Fatalf("UserSetting() error = %v, want nil", err)
	}
	if got != "es-ES" {
		t.Errorf("value = %q, want the stored choice back", got)
	}
}

func TestUserSettingAnswersEmptyWhenUnset(t *testing.T) {
	t.Parallel()

	store := postgres.NewUserSettingStore(newTestPool(t))

	got, err := store.UserSetting(t.Context(), uuid.Must(uuid.NewV7()), "locale.default")

	if err != nil {
		t.Fatalf("UserSetting() error = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("value = %q, want empty for an account that chose nothing", got)
	}
}

func TestUserSettingKeepsOnlyTheLastWrite(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserSettingStore(pool)
	user := seedSettingsUser(t, pool)

	if err := store.SetUserSetting(t.Context(), user, "locale.default", "es-ES"); err != nil {
		t.Fatalf("SetUserSetting() error = %v, want nil", err)
	}
	if err := store.SetUserSetting(t.Context(), user, "locale.default", "en-US"); err != nil {
		t.Fatalf("SetUserSetting() again error = %v, want nil", err)
	}

	got, err := store.UserSetting(t.Context(), user, "locale.default")
	if err != nil {
		t.Fatalf("UserSetting() error = %v, want nil", err)
	}
	if got != "en-US" {
		t.Errorf("value = %q, want the later write to win", got)
	}
}
