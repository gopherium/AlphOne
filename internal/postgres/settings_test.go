// SPDX-License-Identifier: Elastic-2.0

package postgres_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/encryption"
	"github.com/gopherium/alphone/internal/postgres"
	"github.com/gopherium/alphone/sdk"
)

// settingsVersion is the migration creating the workspace settings.
const settingsVersion = 25

// settingKeys answers the encryption keys whose current key is filled with current, the previous ones with previous.
func settingKeys(t *testing.T, current byte, previous ...byte) *encryption.Keys {
	t.Helper()
	held := make([][]byte, 0, len(previous))
	for _, fill := range previous {
		held = append(held, bytes.Repeat([]byte{fill}, encryption.KeySize))
	}
	keys, err := encryption.New(bytes.Repeat([]byte{current}, encryption.KeySize), held)
	if err != nil {
		t.Fatalf("encryption.New() error = %v, want nil", err)
	}
	return keys
}

// noSettingKeys answers encryption keys that hold no key at all.
func noSettingKeys(t *testing.T) *encryption.Keys {
	t.Helper()
	keys, err := encryption.New(nil, nil)
	if err != nil {
		t.Fatalf("encryption.New() error = %v, want nil", err)
	}
	return keys
}

// readBack is what one read of a setting answered.
type readBack struct {
	value string
	found bool
	err   error
}

// String answers the read as its value, whether it was found and its error.
func (r readBack) String() string {
	return fmt.Sprintf("%q found %v error %v", r.value, r.found, r.err)
}

// settingsIn reads and writes one settings store in the workspace ctx serves.
type settingsIn struct {
	t     *testing.T
	ctx   context.Context
	store sdk.SettingStore
}

// save saves values, failing the test on an error.
func (s settingsIn) save(values map[string]string) {
	s.t.Helper()
	if err := s.store.Save(s.ctx, values); err != nil {
		s.t.Fatalf("Save(%v) error = %v, want nil", values, err)
	}
}

// saveSecrets saves secrets, failing the test on an error.
func (s settingsIn) saveSecrets(secrets map[string]string) {
	s.t.Helper()
	if err := s.store.SaveSecrets(s.ctx, secrets); err != nil {
		s.t.Fatalf("SaveSecrets() error = %v, want nil", err)
	}
}

// lookup answers what Lookup reads under key.
func (s settingsIn) lookup(key string) readBack {
	value, found, err := s.store.Lookup(s.ctx, key)
	return readBack{value: value, found: found, err: err}
}

// lookupSecret answers what LookupSecret reads under key.
func (s settingsIn) lookupSecret(key string) readBack {
	value, found, err := s.store.LookupSecret(s.ctx, key)
	return readBack{value: value, found: found, err: err}
}

// wantPlain fails the test unless key reads want as found.
func (s settingsIn) wantPlain(key, want string) {
	s.t.Helper()
	if got := s.lookup(key); got != (readBack{value: want, found: true}) {
		s.t.Errorf("Lookup(%q) = %+v, want %q found", key, got, want)
	}
}

// wantUnset fails the test unless key reads as unset without an error.
func (s settingsIn) wantUnset(key string) {
	s.t.Helper()
	if got := s.lookup(key); got != (readBack{}) {
		s.t.Errorf("Lookup(%q) = %+v, want it unset", key, got)
	}
}

// wantSecret fails the test unless key opens want as found.
func (s settingsIn) wantSecret(key, want string) {
	s.t.Helper()
	if got := s.lookupSecret(key); got != (readBack{value: want, found: true}) {
		s.t.Errorf("LookupSecret(%q) = %+v, want %q found", key, got, want)
	}
}

// wantNoSecret fails the test unless key holds no secret, answered without an error.
func (s settingsIn) wantNoSecret(key string) {
	s.t.Helper()
	if got := s.lookupSecret(key); got != (readBack{}) {
		s.t.Errorf("LookupSecret(%q) = %+v, want it unset", key, got)
	}
}

// wantHasSecret fails the test unless HasSecret answers want for key without an error.
func (s settingsIn) wantHasSecret(key string, want bool) {
	s.t.Helper()
	held, err := s.store.HasSecret(s.ctx, key)
	if err != nil {
		s.t.Errorf("HasSecret(%q) error = %v, want nil", key, err)
	}
	if held != want {
		s.t.Errorf("HasSecret(%q) = %v, want %v", key, held, want)
	}
}

// wantListed fails the test unless LookupPrefix answers want under prefix without an error.
func (s settingsIn) wantListed(prefix string, want map[string]string) {
	s.t.Helper()
	got, err := s.store.LookupPrefix(s.ctx, prefix)
	if err != nil {
		s.t.Errorf("LookupPrefix(%q) error = %v, want nil", prefix, err)
	}
	if !maps.Equal(got, want) {
		s.t.Errorf("LookupPrefix(%q) = %v, want %v", prefix, got, want)
	}
}

// wantTurnedDown fails the test unless a read answered target and nothing beside it.
func wantTurnedDown(t *testing.T, got readBack, target error) {
	t.Helper()
	if !errors.Is(got.err, target) {
		t.Errorf("error = %v, want %v", got.err, target)
	}
	if got.value != "" || got.found {
		t.Errorf("read %q, found %v, want nothing beside the error", got.value, got.found)
	}
}

// settingView names one view of the settings a case runs over.
type settingView string

// The views every case runs over.
const (
	coreView   settingView = "core"
	pluginView settingView = "plugin"
)

// viewOf answers the settings store of the view over a fresh database, read outside any workspace.
func viewOf(t *testing.T, view settingView) settingsIn {
	t.Helper()
	store := postgres.NewSettingStore(newTestPool(t), settingKeys(t, 1))
	if view == pluginView {
		return settingsIn{t: t, ctx: t.Context(), store: store.ForPlugin("email")}
	}
	return settingsIn{t: t, ctx: t.Context(), store: store}
}

// settingCase is one case a settings store passes in every view.
type settingCase struct {
	name string
	run  func(s settingsIn)
}

// settingCases are the cases every settings store passes, over the keys a mail plugin keeps.
var settingCases = []settingCase{
	{"ReadsWhatItWrote", settingReadsWhatItWrote},
	{"ReportsAnUnsetKeyAsUnset", settingReportsAnUnsetKeyAsUnset},
	{"TellsAnEmptyValueFromAnUnsetKey", settingTellsAnEmptyValueFromAnUnsetKey},
	{"OverwritesAnExistingKey", settingOverwritesAnExistingKey},
	{"KeepsKeysApart", settingKeepsKeysApart},
	{"StoresSeveralKeysAsOneWrite", settingStoresSeveralKeysAsOneWrite},
	{"ClearsAKey", settingClearsAKey},
	{"MatchesKeysExactly", settingMatchesKeysExactly},
	{"SavesNothingFromNoValues", settingSavesNothingFromNoValues},
}

// settingReadsWhatItWrote reads back as found the value it stored under a key.
func settingReadsWhatItWrote(s settingsIn) {
	s.save(map[string]string{"imap.host": "imap.example.com"})

	s.wantPlain("imap.host", "imap.example.com")
}

// settingReportsAnUnsetKeyAsUnset reads a key nobody stored as unset and empty, without an error.
func settingReportsAnUnsetKeyAsUnset(s settingsIn) {
	s.wantUnset("imap.host")
}

// settingTellsAnEmptyValueFromAnUnsetKey reads a key stored as empty as found.
func settingTellsAnEmptyValueFromAnUnsetKey(s settingsIn) {
	s.save(map[string]string{"imap.folder": ""})

	s.wantPlain("imap.folder", "")
}

// settingOverwritesAnExistingKey reads back the later of two values written to one key.
func settingOverwritesAnExistingKey(s settingsIn) {
	s.save(map[string]string{"imap.host": "mail.example.com"})

	s.save(map[string]string{"imap.host": "imap.example.com"})

	s.wantPlain("imap.host", "imap.example.com")
}

// settingKeepsKeysApart leaves one key as it was when a write names another.
func settingKeepsKeysApart(s settingsIn) {
	s.save(map[string]string{"imap.host": "imap.example.com"})

	s.save(map[string]string{"imap.port": "993"})

	s.wantPlain("imap.host", "imap.example.com")
}

// settingStoresSeveralKeysAsOneWrite stores every value of a write that names several keys.
func settingStoresSeveralKeysAsOneWrite(s settingsIn) {
	values := map[string]string{"imap.host": "imap.example.com", "imap.port": "993"}

	s.save(values)

	for key, want := range values {
		s.wantPlain(key, want)
	}
}

// settingClearsAKey reads a key back as empty once an empty value is written over it.
func settingClearsAKey(s settingsIn) {
	s.save(map[string]string{"imap.host": "imap.example.com"})
	s.wantPlain("imap.host", "imap.example.com")

	s.save(map[string]string{"imap.host": ""})

	s.wantPlain("imap.host", "")
}

// settingMatchesKeysExactly finds nothing under a key that differs from a stored one in case or a trailing space.
func settingMatchesKeysExactly(s settingsIn) {
	s.save(map[string]string{"imap.host": "imap.example.com"})
	s.wantPlain("imap.host", "imap.example.com")

	for _, near := range []string{"Imap.Host", "imap.host "} {
		s.wantUnset(near)
	}
}

// settingSavesNothingFromNoValues takes a write of no values without an error and leaves every stored key alone.
func settingSavesNothingFromNoValues(s settingsIn) {
	s.save(map[string]string{"imap.host": "imap.example.com"})

	for _, empty := range []map[string]string{nil, {}} {
		s.save(empty)
		s.saveSecrets(empty)
	}

	s.wantPlain("imap.host", "imap.example.com")
}

func TestEverySettingsViewPassesTheStoreCases(t *testing.T) {
	t.Parallel()

	for _, view := range []settingView{coreView, pluginView} {
		for _, tc := range settingCases {
			t.Run(string(view)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				tc.run(viewOf(t, view))
			})
		}
	}
}

func TestAValueSavedInOneWorkspaceIsUnsetInAnother(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewSettingStore(pool, settingKeys(t, 1)).ForPlugin("email")
	acme := settingsIn{t: t, ctx: standingIn(t, seededTenant(t, pool)), store: store}
	elsewhere := settingsIn{t: t, ctx: t.Context(), store: store}
	acme.save(map[string]string{"imap.host": "imap.example.com"})
	acme.saveSecrets(map[string]string{"imap.password": "correct horse battery"})

	elsewhere.wantUnset("imap.host")
	elsewhere.wantNoSecret("imap.password")
	elsewhere.wantListed("imap.", map[string]string{})
	acme.wantPlain("imap.host", "imap.example.com")
}

func TestAPluginReachesNeitherAnotherPluginsKeysNorTheCores(t *testing.T) {
	t.Parallel()

	store := postgres.NewSettingStore(newTestPool(t), settingKeys(t, 1))
	core := settingsIn{t: t, ctx: t.Context(), store: store}
	email := settingsIn{t: t, ctx: t.Context(), store: store.ForPlugin("email")}
	extra := settingsIn{t: t, ctx: t.Context(), store: store.ForPlugin("email-extra")}
	core.save(map[string]string{"imap.host": "core.example.com"})
	email.save(map[string]string{"imap.host": "imap.example.com"})
	extra.save(map[string]string{"imap.host": "extra.example.com"})

	email.wantPlain("imap.host", "imap.example.com")
	extra.wantPlain("imap.host", "extra.example.com")
	core.wantPlain("imap.host", "core.example.com")
	core.wantPlain("plugin.email.imap.host", "imap.example.com")
	settingsIn{t: t, ctx: t.Context(), store: store.ForPlugin("calendar")}.wantUnset("imap.host")
	email.wantListed("", map[string]string{"imap.host": "imap.example.com"})
}

// storeHandingOutStores is a settings store that hands out the store of any plugin.
type storeHandingOutStores interface {
	ForPlugin(id string) sdk.SettingStore
}

func TestAPluginStoreHandsOutNoStoreOfAnotherPlugin(t *testing.T) {
	t.Parallel()

	store := postgres.NewSettingStore(newTestPool(t), settingKeys(t, 1))

	if _, handing := store.ForPlugin("email").(storeHandingOutStores); handing {
		t.Error("the email store hands out the store of any plugin, want it to keep its own keys alone")
	}
}

func TestAPlainReadAndASecretReadEachTurnDownTheOtherKind(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)
	s.save(map[string]string{"imap.host": "imap.example.com"})
	s.saveSecrets(map[string]string{"imap.password": "correct horse battery"})

	wantTurnedDown(t, s.lookup("imap.password"), sdk.ErrSecretSetting)
	wantTurnedDown(t, s.lookupSecret("imap.host"), sdk.ErrPlainSetting)
	held, err := s.store.HasSecret(t.Context(), "imap.host")
	wantTurnedDown(t, readBack{found: held, err: err}, sdk.ErrPlainSetting)
	s.wantSecret("imap.password", "correct horse battery")
}

// copied names the workspace and the full key a test copies an encrypted value from and to.
type copied struct {
	from, to  uuid.UUID
	key, into string
}

// copySecret stores the encrypted value the copy names under its other workspace and key, by hand.
func copySecret(t *testing.T, pool *pgxpool.Pool, moved copied) {
	t.Helper()
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO core.settings (tenant_id, key, encrypted)
		SELECT $1, $2, encrypted FROM core.settings WHERE tenant_id = $3 AND key = $4`,
		moved.to, moved.into, moved.from, moved.key); err != nil {
		t.Fatalf("copying the encrypted value: %v", err)
	}
}

func TestAnEncryptedValueCopiedElsewhereDoesNotOpen(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewSettingStore(pool, settingKeys(t, 1))
	acme := seededTenant(t, pool)
	mine := settingsIn{t: t, ctx: standingIn(t, acme), store: store.ForPlugin("email")}
	mine.saveSecrets(map[string]string{"imap.password": "correct horse battery"})
	held := "plugin.email.imap.password"
	copySecret(t, pool, copied{from: acme, to: sdk.DefaultTenantID, key: held, into: held})
	copySecret(t, pool, copied{from: acme, to: acme, key: held, into: "plugin.email.smtp.password"})
	copySecret(t, pool, copied{from: acme, to: acme, key: held, into: "plugin.calendar.imap.password"})

	for name, read := range map[string]readBack{
		"another workspace": settingsIn{t: t, ctx: t.Context(), store: mine.store}.lookupSecret("imap.password"),
		"another key":       mine.lookupSecret("smtp.password"),
		"another plugin": settingsIn{
			t: t, ctx: mine.ctx, store: store.ForPlugin("calendar"),
		}.lookupSecret("imap.password"),
	} {
		t.Run(name, func(t *testing.T) {
			wantTurnedDown(t, read, sdk.ErrCannotDecrypt)
		})
	}
	mine.wantSecret("imap.password", "correct horse battery")
}

// storedEncrypted answers the encrypted value the workspace holds under the full key.
func storedEncrypted(t *testing.T, pool *pgxpool.Pool, tenant uuid.UUID, key string) []byte {
	t.Helper()
	var encrypted []byte
	if err := pool.QueryRow(t.Context(), "SELECT encrypted FROM core.settings WHERE tenant_id = $1 AND key = $2",
		tenant, key).Scan(&encrypted); err != nil {
		t.Fatalf("reading the encrypted value under %s: %v", key, err)
	}
	return encrypted
}

func TestAStoredSecretIsBoundToTheSettingLabelItsWorkspaceAndItsFullKey(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	acme := seededTenant(t, pool)
	store := postgres.NewSettingStore(pool, settingKeys(t, 1)).ForPlugin("email")
	settingsIn{t: t, ctx: standingIn(t, acme), store: store}.
		saveSecrets(map[string]string{"imap.password": "correct horse battery"})

	plaintext, err := settingKeys(t, 1).Decrypt(storedEncrypted(t, pool, acme, "plugin.email.imap.password"),
		[]byte("setting"), acme[:], []byte("plugin.email.imap.password"))

	if err != nil || string(plaintext) != "correct horse battery" {
		t.Errorf("Decrypt() with the setting binding = %q, %v, want the secret saved", plaintext, err)
	}
}

func TestAPlainValueAndASecretTakeEachOthersPlace(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)
	s.saveSecrets(map[string]string{"imap.password": "correct horse battery"})

	s.save(map[string]string{"imap.password": "written in plain"})
	s.wantPlain("imap.password", "written in plain")

	s.saveSecrets(map[string]string{"imap.password": "encrypted again"})
	s.wantSecret("imap.password", "encrypted again")
}

func TestAnEmptySecretReadsFoundAndCountsAsNoSecret(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)
	s.saveSecrets(map[string]string{"imap.password": "", "smtp.password": "correct horse battery"})

	s.wantSecret("imap.password", "")
	s.wantHasSecret("imap.password", false)
	s.wantHasSecret("smtp.password", true)
	s.wantHasSecret("pop.password", false)
}

func TestLookupPrefixReadsThePrefixPlainlyAndLeavesSecretsOut(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)
	s.save(map[string]string{
		"imap_a.host": "underscore.example.com", "imapxa.host": "letter.example.com", "imap.host": "imap.example.com",
	})
	s.saveSecrets(map[string]string{"imap.password": "correct horse battery"})

	s.wantListed("imap_", map[string]string{"imap_a.host": "underscore.example.com"})
	s.wantListed("imap.", map[string]string{"imap.host": "imap.example.com"})
	s.wantListed("%", map[string]string{})
}

// wantKeyFormatCheck fails the test unless err is the key format check of the table naming key.
func wantKeyFormatCheck(t *testing.T, err error, key string) {
	t.Helper()
	if sqlStateOf(err) != checkViolation {
		t.Errorf("error = %v, want the key format check %s", err, checkViolation)
	}
	if !strings.Contains(fmt.Sprint(err), key) {
		t.Errorf("error = %v, want it to name %q", err, key)
	}
}

func TestAWriteWithOneBadKeyStoresNoneOfItsValues(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)

	wantKeyFormatCheck(t, s.store.Save(t.Context(), map[string]string{
		"imap.host": "imap.example.com", "smtp.Port": "587",
	}), "smtp.Port")
	wantKeyFormatCheck(t, s.store.SaveSecrets(t.Context(), map[string]string{
		"imap.password": "correct horse battery", "smtp.Password": "x",
	}), "smtp.Password")

	s.wantUnset("imap.host")
	s.wantListed("", map[string]string{})
	s.wantHasSecret("imap.password", false)
}

func TestAWriteStopsAtTheFirstBadKeyInSortedOrder(t *testing.T) {
	t.Parallel()

	s := viewOf(t, pluginView)
	bad := map[string]string{"smtp.Port": "587", "imap.Host": "imap.example.com"}

	for range 20 {
		for name, err := range map[string]error{
			"Save":        s.store.Save(t.Context(), bad),
			"SaveSecrets": s.store.SaveSecrets(t.Context(), bad),
		} {
			if got := fmt.Sprint(err); !strings.Contains(got, "imap.Host") || strings.Contains(got, "smtp.Port") {
				t.Fatalf("%s() error = %v, want it to name imap.Host, the first bad key in sorted order", name, err)
			}
		}
	}
}

func TestWithoutAnEncryptionKeySecretsAnswerTheMissingKey(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	settingsIn{t: t, ctx: t.Context(), store: postgres.NewSettingStore(pool, settingKeys(t, 1))}.
		saveSecrets(map[string]string{"imap.password": "correct horse battery"})
	keyless := settingsIn{t: t, ctx: t.Context(), store: postgres.NewSettingStore(pool, noSettingKeys(t))}

	err := keyless.store.SaveSecrets(t.Context(), map[string]string{"smtp.password": "correct horse battery"})

	wantTurnedDown(t, readBack{err: err}, sdk.ErrNoEncryptionKey)
	if !strings.Contains(fmt.Sprint(err), "ALPHONE_ENCRYPTION_KEY") {
		t.Errorf("SaveSecrets() error = %v, want it to name ALPHONE_ENCRYPTION_KEY", err)
	}
	wantTurnedDown(t, keyless.lookupSecret("imap.password"), sdk.ErrNoEncryptionKey)
	keyless.save(map[string]string{"imap.host": "imap.example.com"})
	keyless.wantPlain("imap.host", "imap.example.com")
}

func TestASecretSavedUnderAnOlderKeyOpensOnceTheKeyIsReplaced(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	settingsIn{t: t, ctx: t.Context(), store: postgres.NewSettingStore(pool, settingKeys(t, 1))}.
		saveSecrets(map[string]string{"imap.password": "older secret"})
	rotated := settingsIn{t: t, ctx: t.Context(), store: postgres.NewSettingStore(pool, settingKeys(t, 2, 1))}

	rotated.saveSecrets(map[string]string{"smtp.password": "newer secret"})

	rotated.wantSecret("imap.password", "older secret")
	settingsIn{t: t, ctx: t.Context(), store: postgres.NewSettingStore(pool, settingKeys(t, 2))}.
		wantSecret("smtp.password", "newer secret")
}

func TestTheSettingStoreReportsAClosedPool(t *testing.T) {
	t.Parallel()

	pool := newTestPool(t)
	store := postgres.NewSettingStore(pool, settingKeys(t, 1))
	pool.Close()

	s := settingsIn{t: t, ctx: t.Context(), store: store}
	_, hasErr := store.HasSecret(t.Context(), "imap.password")
	_, listErr := store.LookupPrefix(t.Context(), "imap.")

	for name, err := range map[string]error{
		"Lookup":       s.lookup("imap.host").err,
		"LookupSecret": s.lookupSecret("imap.password").err,
		"HasSecret":    hasErr,
		"LookupPrefix": listErr,
		"Save":         store.Save(t.Context(), map[string]string{"imap.host": "x"}),
		"SaveSecrets":  store.SaveSecrets(t.Context(), map[string]string{"imap.password": "x"}),
	} {
		if err == nil {
			t.Errorf("%s() error = nil, want the closed pool reported", name)
		}
	}
	for name, err := range map[string]error{
		"Save() of no values":        store.Save(t.Context(), nil),
		"SaveSecrets() of no values": store.SaveSecrets(t.Context(), map[string]string{}),
	} {
		if err != nil {
			t.Errorf("%s error = %v, want nil with nothing to write", name, err)
		}
	}
}

func TestTheSettingsTableHoldsAPlainValueOrASecretNeverBoth(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	tests := map[string]struct {
		value     any
		encrypted any
		wantCode  string
	}{
		"a plain value":        {"imap.example.com", nil, ""},
		"a secret":             {nil, []byte{1}, ""},
		"neither of the two":   {nil, nil, checkViolation},
		"both of them at once": {"imap.example.com", []byte{1}, checkViolation},
	}

	for name, tt := range tests {
		_, err := db.Exec(`INSERT INTO core.settings (tenant_id, key, value, encrypted) VALUES ($1, $2, $3, $4)`,
			sdk.DefaultTenantID, "check."+strings.ReplaceAll(name, " ", "_"), tt.value, tt.encrypted)
		if got := sqlStateOf(err); got != tt.wantCode {
			t.Errorf("storing %s answered %q (%v), want %q", name, got, err, tt.wantCode)
		}
	}
}

func TestTheSettingsTableAdmitsTheKeyFormsBothProductsUse(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	tests := map[string]string{
		"locale.default": "", "site_id": "", "plugin.demo-notes.limit": "", "plugin.email.imap.host": "",
		"Locale.default": checkViolation, "locale.": checkViolation, ".locale": checkViolation,
		"locale..default": checkViolation, "1locale": checkViolation, "locale default": checkViolation, "": checkViolation,
	}

	for key, wantCode := range tests {
		_, err := db.Exec(`INSERT INTO core.settings (tenant_id, key, value) VALUES ($1, $2, 'x')`,
			sdk.DefaultTenantID, key)
		if got := sqlStateOf(err); got != wantCode {
			t.Errorf("storing the key %q answered %q (%v), want %q", key, got, err, wantCode)
		}
	}
}

func TestEveryWorkspaceSettingNamesAWorkspaceThatExists(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)

	_, err := db.Exec(`INSERT INTO core.settings (tenant_id, key, value) VALUES ($1, 'imap.host', 'x')`,
		uuid.Must(uuid.NewV7()))

	if got := sqlStateOf(err); got != foreignKeyViolation {
		t.Errorf("storing a setting of no workspace answered %q (%v), want %q", got, err, foreignKeyViolation)
	}
}

func TestRollingTheSettingsMigrationBackDropsTheTable(t *testing.T) {
	t.Parallel()

	db := newTestDB(t)
	if !settingsTableExists(t, db) {
		t.Fatal("core.settings is missing before the rollback, want the migration to create it")
	}

	if _, err := coreProvider(t, db).DownTo(t.Context(), settingsVersion-1); err != nil {
		t.Fatalf("rolling back the settings migration: %v", err)
	}

	if settingsTableExists(t, db) {
		t.Error("core.settings is still there after the rollback, want it dropped")
	}
}

// settingsTableExists reports whether db holds core.settings.
func settingsTableExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var held bool
	if err := db.QueryRow("SELECT to_regclass('core.settings') IS NOT NULL").Scan(&held); err != nil {
		t.Fatalf("looking for core.settings: %v", err)
	}
	return held
}
