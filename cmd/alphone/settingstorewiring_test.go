// SPDX-License-Identifier: Elastic-2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gopherium/alphone/internal/encryption"
	"github.com/gopherium/alphone/sdk"
)

// currentEncryptionKey is the encryption key the settings tests set, written as openssl rand -hex 32 prints one.
const currentEncryptionKey = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

// olderEncryptionKey is the encryption key the settings tests replace, written the same way.
const olderEncryptionKey = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"

// settingTakingPlugin records the settings store and the encrypter the host hands it.
type settingTakingPlugin struct {
	inertPlugin
	settings  sdk.SettingStore
	encrypter sdk.Encrypter
}

// UseSettingStore keeps the handed store.
func (p *settingTakingPlugin) UseSettingStore(store sdk.SettingStore) {
	p.settings = store
}

// UseEncrypter keeps the handed encrypter.
func (p *settingTakingPlugin) UseEncrypter(encrypter sdk.Encrypter) {
	p.encrypter = encrypter
}

// settingRead is what one read of a setting or one decryption answered.
type settingRead struct {
	value string
	found bool
	err   error
}

// String answers the read as its value, whether it was found and its error.
func (r settingRead) String() string {
	return fmt.Sprintf("%q found %v error %v", r.value, r.found, r.err)
}

// readPlain answers what store reads as a plain value under key in the workspace ctx serves.
func readPlain(ctx context.Context, store sdk.SettingStore, key string) settingRead {
	value, found, err := store.Lookup(ctx, key)
	return settingRead{value: value, found: found, err: err}
}

// readSecret answers what store opens as a secret under key in the workspace ctx serves.
func readSecret(ctx context.Context, store sdk.SettingStore, key string) settingRead {
	value, found, err := store.LookupSecret(ctx, key)
	return settingRead{value: value, found: found, err: err}
}

// decrypted answers what one decryption answered, found when it opened.
func decrypted(plaintext []byte, err error) settingRead {
	return settingRead{value: string(plaintext), found: plaintext != nil, err: err}
}

// foundValue answers the read of value as found without an error.
func foundValue(value string) settingRead {
	return settingRead{value: value, found: true}
}

// composedSettingTakers composes the host over env with one setting taking plugin under each id beside a bystander.
func composedSettingTakers(t *testing.T, env map[string]string, ids ...string) []*settingTakingPlugin {
	t.Helper()
	taking := make([]*settingTakingPlugin, 0, len(ids))
	registered := []sdk.Plugin{inertPlugin{id: "bystander"}}
	for _, id := range ids {
		plugin := &settingTakingPlugin{inertPlugin: inertPlugin{id: id}}
		taking, registered = append(taking, plugin), append(registered, plugin)
	}
	built, err := compose(t.Context(), composeConfigOf(t, env), func(sdk.Deps) ([]sdk.Plugin, error) {
		return registered, nil
	})
	if err != nil {
		t.Fatalf("compose() error = %v, want nil", err)
	}
	abandonAtEnd(t, built)
	for _, plugin := range taking {
		wantBothSeams(t, plugin)
	}
	return taking
}

// wantBothSeams fails the test unless the plugin received a settings store and an encrypter.
func wantBothSeams(t *testing.T, plugin *settingTakingPlugin) {
	t.Helper()
	if plugin.settings == nil || plugin.encrypter == nil {
		t.Fatalf("the plugin %s received %v and %v, want a settings store and an encrypter", plugin.id,
			plugin.settings, plugin.encrypter)
	}
}

// keyedSettings returns the settings that point at databaseURL with the keys env adds.
func keyedSettings(databaseURL string, env map[string]string) map[string]string {
	settings := map[string]string{"ALPHONE_DATABASE_URL": databaseURL}
	maps.Copy(settings, env)
	return settings
}

// currentKeyOnly returns the settings that set the current encryption key alone.
func currentKeyOnly() map[string]string {
	return map[string]string{"ALPHONE_ENCRYPTION_KEY": currentEncryptionKey}
}

// wantRead fails the test unless the read named what answered want.
func wantRead(t *testing.T, what string, got, want settingRead) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %+v, want %+v", what, got, want)
	}
}

// wantCannotOpen fails the test unless the read named what answered ErrCannotDecrypt and nothing beside it.
func wantCannotOpen(t *testing.T, what string, got settingRead) {
	t.Helper()
	if !errors.Is(got.err, sdk.ErrCannotDecrypt) || got != (settingRead{err: got.err}) {
		t.Errorf("%s = %+v, want nothing beside %v", what, got, sdk.ErrCannotDecrypt)
	}
}

// settingsPool answers a pool on the database at databaseURL, closed once the test ends.
func settingsPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connecting pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// storeEncrypted stores encrypted under the full key of the default workspace, by hand.
func storeEncrypted(t *testing.T, pool *pgxpool.Pool, key string, encrypted []byte) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), "INSERT INTO core.settings (tenant_id, key, encrypted) VALUES ($1, $2, $3)",
		sdk.DefaultTenantID, key, encrypted); err != nil {
		t.Fatalf("storing the encrypted value under %s: %v", key, err)
	}
}

// storedEncrypted answers the encrypted value the default workspace holds under the full key.
func storedEncrypted(t *testing.T, pool *pgxpool.Pool, key string) []byte {
	t.Helper()
	var encrypted []byte
	if err := pool.QueryRow(t.Context(), "SELECT encrypted FROM core.settings WHERE tenant_id = $1 AND key = $2",
		sdk.DefaultTenantID, key).Scan(&encrypted); err != nil {
		t.Fatalf("reading the encrypted value under %s: %v", key, err)
	}
	return encrypted
}

// currentKeys answers the encryption keys over the current key the settings tests set.
func currentKeys(t *testing.T) *encryption.Keys {
	t.Helper()
	raw, err := parseEncryptionKey(currentEncryptionKey)
	if err != nil {
		t.Fatalf("parseEncryptionKey() error = %v, want nil", err)
	}
	keys, err := encryption.New(raw, nil)
	if err != nil {
		t.Fatalf("encryption.New() error = %v, want nil", err)
	}
	return keys
}

// wantMissingKey fails the test unless err is the missing key error naming the setting the server reads.
func wantMissingKey(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, sdk.ErrNoEncryptionKey) {
		t.Errorf("%s error = %v, want %v", what, err, sdk.ErrNoEncryptionKey)
	}
	if want := settingsEnv(nil).Key(encryptionKeySetting); !strings.Contains(fmt.Sprint(err), want) {
		t.Errorf("%s error = %v, want it to name %s", what, err, want)
	}
}

func TestComposeHandsEachSettingTakingPluginAStoreOfItsOwn(t *testing.T) {
	t.Parallel()

	taking := composedSettingTakers(t, keyedSettings(migratedPostgres(t), currentKeyOnly()), "email", "calendar")
	email, calendar := taking[0].settings, taking[1].settings

	if err := email.Save(t.Context(), map[string]string{"imap.host": "imap.example.com"}); err != nil {
		t.Fatalf("Save() through the email store error = %v, want nil", err)
	}
	if err := email.SaveSecrets(t.Context(), map[string]string{"imap.password": "correct horse battery"}); err != nil {
		t.Fatalf("SaveSecrets() through the email store error = %v, want nil", err)
	}

	wantRead(t, "Lookup() through the calendar store", readPlain(t.Context(), calendar, "imap.host"), settingRead{})
	wantRead(t, "Lookup() through the email store", readPlain(t.Context(), email, "imap.host"),
		foundValue("imap.example.com"))
	wantRead(t, "LookupSecret() through the email store", readSecret(t.Context(), email, "imap.password"),
		foundValue("correct horse battery"))
}

func TestComposeHandsEachPluginAnEncrypterBoundToItsWorkspaceAndPurpose(t *testing.T) {
	t.Parallel()

	taking := composedSettingTakers(t, keyedSettings(unreachableDatabaseURL, currentKeyOnly()), "email", "calendar")
	email, calendar := taking[0].encrypter, taking[1].encrypter
	acme := sdk.WithTenant(t.Context(), uuid.Must(uuid.NewV7()))
	ciphertext, err := email.Encrypt(acme, "imap.password", []byte("correct horse battery"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v, want nil", err)
	}

	wantRead(t, "Decrypt() in the same workspace", decrypted(email.Decrypt(acme, "imap.password", ciphertext)),
		foundValue("correct horse battery"))
	for name, got := range map[string]settingRead{
		"another workspace": decrypted(email.Decrypt(t.Context(), "imap.password", ciphertext)),
		"another purpose":   decrypted(email.Decrypt(acme, "smtp.password", ciphertext)),
		"another plugin":    decrypted(calendar.Decrypt(acme, "imap.password", ciphertext)),
	} {
		wantCannotOpen(t, "Decrypt() for "+name, got)
	}
}

func TestAPluginValueIsBoundToThePluginLabelItsWorkspaceItsPluginAndItsPurpose(t *testing.T) {
	t.Parallel()

	email := composedSettingTakers(t, keyedSettings(unreachableDatabaseURL, currentKeyOnly()), "email")[0]
	acme := uuid.Must(uuid.NewV7())

	ciphertext, err := email.encrypter.Encrypt(sdk.WithTenant(t.Context(), acme), "imap.password",
		[]byte("correct horse battery"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v, want nil", err)
	}

	wantRead(t, "Decrypt() with the plugin binding",
		decrypted(currentKeys(t).Decrypt(ciphertext, []byte("plugin"), acme[:], []byte("email"), []byte("imap.password"))),
		foundValue("correct horse battery"))
}

func TestAPluginValueAndAStoredSecretDoNotOpenAsEachOther(t *testing.T) {
	t.Parallel()

	databaseURL := migratedPostgres(t)
	email := composedSettingTakers(t, keyedSettings(databaseURL, currentKeyOnly()), "email")[0]
	pool := settingsPool(t, databaseURL)
	pluginValue, err := email.encrypter.Encrypt(t.Context(), "imap.password", []byte("correct horse battery"))
	if err != nil {
		t.Fatalf("Encrypt() error = %v, want nil", err)
	}
	secrets := map[string]string{"smtp.password": "correct horse battery"}
	if err := email.settings.SaveSecrets(t.Context(), secrets); err != nil {
		t.Fatalf("SaveSecrets() error = %v, want nil", err)
	}

	storeEncrypted(t, pool, "plugin.email.imap.password", pluginValue)
	storedSecret := storedEncrypted(t, pool, "plugin.email.smtp.password")

	wantCannotOpen(t, "LookupSecret() of a plugin value stored as a setting",
		readSecret(t.Context(), email.settings, "imap.password"))
	wantCannotOpen(t, "Decrypt() of a stored secret",
		decrypted(email.encrypter.Decrypt(t.Context(), "smtp.password", storedSecret)))
}

func TestWithoutAnEncryptionKeySecretsNameTheMissingVariable(t *testing.T) {
	t.Parallel()

	taking := composedSettingTakers(t, keyedSettings(migratedPostgres(t), nil), "email")[0]

	wantMissingKey(t, "SaveSecrets()",
		taking.settings.SaveSecrets(t.Context(), map[string]string{"imap.password": "correct horse battery"}))
	_, err := taking.encrypter.Encrypt(t.Context(), "imap.password", []byte("correct horse battery"))
	wantMissingKey(t, "Encrypt()", err)
	_, err = taking.encrypter.Decrypt(t.Context(), "imap.password", []byte{1})
	wantMissingKey(t, "Decrypt()", err)
	if err := taking.settings.Save(t.Context(), map[string]string{"imap.host": "imap.example.com"}); err != nil {
		t.Errorf("Save() of a plain value error = %v, want it stored without a key", err)
	}
}

func TestAReplacedEncryptionKeyStillOpensTheOlderSecrets(t *testing.T) {
	t.Parallel()

	databaseURL := migratedPostgres(t)
	older := composedSettingTakers(t, keyedSettings(databaseURL, map[string]string{
		"ALPHONE_ENCRYPTION_KEY": olderEncryptionKey,
	}), "email")[0]
	if err := older.settings.SaveSecrets(t.Context(), map[string]string{"smtp.password": "older secret"}); err != nil {
		t.Fatalf("SaveSecrets() under the older key error = %v, want nil", err)
	}
	ciphertext, err := older.encrypter.Encrypt(t.Context(), "access_token", []byte("older token"))
	if err != nil {
		t.Fatalf("Encrypt() under the older key error = %v, want nil", err)
	}

	rotated := composedSettingTakers(t, keyedSettings(databaseURL, map[string]string{
		"ALPHONE_ENCRYPTION_KEY": currentEncryptionKey, "ALPHONE_ENCRYPTION_PREVIOUS_KEYS": olderEncryptionKey,
	}), "email")[0]

	wantRead(t, "LookupSecret() after the rotation", readSecret(t.Context(), rotated.settings, "smtp.password"),
		foundValue("older secret"))
	wantRead(t, "Decrypt() after the rotation",
		decrypted(rotated.encrypter.Decrypt(t.Context(), "access_token", ciphertext)), foundValue("older token"))
}
