// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"context"
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

// userSettingsOf reads and writes the settings of one user in the workspace ctx serves.
type userSettingsOf struct {
	t     *testing.T
	ctx   context.Context
	store *postgres.UserSettingStore
	user  uuid.UUID
}

// save stores value under key, failing the test on an error.
func (u userSettingsOf) save(key, value string) {
	u.t.Helper()
	if err := u.store.Save(u.ctx, u.user, key, value); err != nil {
		u.t.Fatalf("Save(%q, %q) error = %v, want nil", key, value, err)
	}
}

// want fails the test unless key reads back as want says.
func (u userSettingsOf) want(key string, want readBack) {
	u.t.Helper()
	held, found, err := u.store.Lookup(u.ctx, u.user, key)
	if got := (readBack{value: held, found: found, err: err}); got != want {
		u.t.Errorf("Lookup(%q) = %+v, want %+v", key, got, want)
	}
}

// in answers the same user's settings in the workspace ctx serves.
func (u userSettingsOf) in(ctx context.Context) userSettingsOf {
	u.ctx = ctx
	return u
}

// readerOver answers the settings of one new user over pool, read outside any workspace.
func readerOver(t *testing.T, pool *pgxpool.Pool) userSettingsOf {
	t.Helper()
	return userSettingsOf{
		t: t, ctx: t.Context(), store: postgres.NewUserSettingStore(pool), user: seedSettingsUser(t, pool),
	}
}

// foundAs answers the read of a value stored as value.
func foundAs(value string) readBack {
	return readBack{value: value, found: true}
}

func TestAUserSettingReadsWhatItWrote(t *testing.T) {
	t.Parallel()

	reader := readerOver(t, newTestPool(t))

	reader.save("locale.default", "es-ES")

	reader.want("locale.default", foundAs("es-ES"))
}

func TestAUserSettingCarriesTheLatestWrite(t *testing.T) {
	t.Parallel()

	reader := readerOver(t, newTestPool(t))
	reader.save("locale.default", "es-ES")

	reader.save("locale.default", "en-US")

	reader.want("locale.default", foundAs("en-US"))
}

func TestAUserSettingIsTheReadersAlone(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	mine, theirs := readerOver(t, pool), readerOver(t, pool)

	mine.save("locale.default", "es-ES")

	theirs.want("locale.default", readBack{})
	mine.want("locale.default", foundAs("es-ES"))
}

func TestAUserSettingAnswersNothingForAKeyItNeverHeld(t *testing.T) {
	t.Parallel()

	userWithNoSettings := userSettingsOf{
		t: t, ctx: t.Context(), store: postgres.NewUserSettingStore(newTestPool(t)), user: uuid.Must(uuid.NewV7()),
	}

	userWithNoSettings.want("locale.default", readBack{})
}

func TestAUserSettingTellsAnEmptyValueFromAnUnsetKey(t *testing.T) {
	t.Parallel()

	reader := readerOver(t, newTestPool(t))

	reader.save("locale.default", "")

	reader.want("locale.default", foundAs(""))
}

func TestAUserSettingKeepsEachKeyApart(t *testing.T) {
	t.Parallel()

	reader := readerOver(t, newTestPool(t))
	for _, write := range []struct{ key, value string }{
		{"locale.default", "es-ES"}, {"list.page_size", "50"}, {"locale.default", "en-US"},
	} {
		reader.save(write.key, write.value)
	}

	reader.want("locale.default", foundAs("en-US"))
	reader.want("list.page_size", foundAs("50"))
}

func TestAUserSettingMatchesKeysExactly(t *testing.T) {
	t.Parallel()

	reader := readerOver(t, newTestPool(t))

	reader.save("locale.default", "es-ES")

	for _, near := range []string{"Locale.Default", "locale.default "} {
		reader.want(near, readBack{})
	}
	reader.want("locale.default", foundAs("es-ES"))
}

func TestTheUserSettingStoreReportsAClosedPool(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewUserSettingStore(pool)
	pool.Close()

	if _, _, err := store.Lookup(t.Context(), uuid.Must(uuid.NewV7()), "locale.default"); err == nil {
		t.Error("Lookup() error = nil, want the closed pool reported")
	}
	if err := store.Save(t.Context(), uuid.Must(uuid.NewV7()), "locale.default", "es-ES"); err == nil {
		t.Error("Save() error = nil, want the closed pool reported")
	}
}
