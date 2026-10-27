---
title: Settings
description: Keep the settings each workspace chooses for your plugin, such as a mailbox address and its password, with the secrets encrypted.
---

A plugin that needs the settings each workspace chooses, such as the
address and the password of a mailbox, gets a settings store from
AlphOne. It saves several values in one write and reads each one back.
The store keeps every value under the workspace and the plugin that
saved it, so no other workspace and no other plugin ever reaches it.

If you know WordPress, this is `get_option` and `update_option`, kept
apart for each workspace.

## Take a store

Implement `sdk.SettingStoreConsumer`. AlphOne calls its one method after
every plugin has registered and before any plugin starts:

```go
var _ sdk.SettingStoreConsumer = (*Plugin)(nil)

// UseSettingStore keeps the settings store AlphOne hands the plugin.
func (p *Plugin) UseSettingStore(store sdk.SettingStore) {
	p.settings = store
}
```

Every plugin that implements it gets a store of its own. The `var _`
line turns a misspelled method into a build error.

## Keys

A key is made of parts joined by dots, and each part holds lower case
letters, digits, `_` and `-`, such as `imap.host` or `imap.password`.
AlphOne keeps your keys under `plugin.` and your plugin id, so the
`imap.host` of the email plugin is stored as `plugin.email.imap.host`.
You never write that part yourself, and your plugin cannot name a key
of another plugin or of AlphOne itself.

A key in another form, such as `Imap.Host` or `imap..host`, fails the
whole write, and nothing of it is stored. Keys come from your code, so a
test of your plugin finds a bad one before anyone else does.

A read matches the key exactly. `Imap.Host`, or `imap.host` with a space
after it, reads as unset.

## Plain values

`Save` stores every value of the map or none of them:

```go
err := p.settings.Save(ctx, map[string]string{
	"imap.host": "imap.example.com",
	"imap.port": "993",
})
```

An empty map stores nothing and answers no error.

`Lookup` answers the value and whether the key holds one:

```go
host, found, err := p.settings.Lookup(ctx, "imap.host")
```

`found` tells an unset key from one stored as empty. An empty value is a
real value. It reads back found and empty. Nothing deletes a key, so
save an empty value to clear one.

`LookupPrefix` answers every plain value whose key starts with a prefix,
such as every key under `imap.`, and leaves the secrets out. It reads the
prefix as plain text, so `_` and `%` match only themselves.

## Secrets

Passwords and tokens go through the secret methods:

```go
err := p.settings.SaveSecrets(ctx, map[string]string{"imap.password": password})

password, found, err := p.settings.LookupSecret(ctx, "imap.password")

set, err := p.settings.HasSecret(ctx, "imap.password")
```

`SaveSecrets` encrypts every secret with the key the operator sets in
`ALPHONE_ENCRYPTION_KEY`, see
[Encryption](/self-hosting/configuration/#encryption), and stores all of
them or none. `HasSecret` answers whether a key holds a secret that is
not empty, so a screen can show that a password is set without ever
sending it to the browser. An empty secret reads back found and empty,
and `HasSecret` answers false for it. `HasSecret` decrypts the secret to
tell, so like `LookupSecret` it answers `sdk.ErrNoEncryptionKey` without
a key, `sdk.ErrCannotDecrypt` when no key opens the secret, and
`sdk.ErrPlainSetting` for a key that holds a plain value.

A plain read never shows a secret. `Lookup` of a key that holds a secret
answers `sdk.ErrSecretSetting`, and `LookupSecret` of a plain value
answers `sdk.ErrPlainSetting`. A write of the other kind replaces what
the key held, so a value can move from plain to secret.

Without a key the server still starts, and plain values work. A secret
write or read then answers `sdk.ErrNoEncryptionKey`, whose text names
`ALPHONE_ENCRYPTION_KEY`. Show it to the person, so they know what to
ask the operator for.

The database cannot search or sort an encrypted value, because it holds
only bytes it cannot read.
[Laravel warns about the same thing](https://laravel.com/docs/13.x/eloquent-mutators#encrypted-casting)
for its encrypted columns. Keep anything you need to search as a plain
value.

## Every call serves one workspace

Each call works in the workspace `ctx` carries. AlphOne puts it on `ctx`
for every signed in request. Work your plugin starts on its own, such as
a background sync, sets it with `sdk.WithTenant`. A `ctx` that carries no
workspace works in the default one.

A value saved in one workspace reads as unset in every other workspace.

## Which value wins

A value the workspace stored always wins, even an empty one. The
environment file only fills in the default workspace, and only while
that workspace stored nothing under the key. Every other workspace gets
your plugin's own default or nothing. The WhatsApp plugin already reads
its credentials this way. For the host of a mailbox, with `p.seed`
holding what the plugin read from the environment at `Register`, it
looks like this:

```go
host, found, err := p.settings.Lookup(ctx, "imap.host")
if err != nil {
	return err
}
if !found && sdk.TenantOrDefault(ctx) == sdk.DefaultTenantID {
	host = p.seed.imapHost
}
```

Nothing copies a value from the environment file into the store. Once a
workspace saves a value, its line in the environment file can go.

For settings like these the environment file is a stopgap, and it
shrinks over time. It keeps what AlphOne needs before the database
opens, such as the database address, what holds for a whole server
process, such as the page bounds, and the encryption keys themselves,
because a key stored next to the data it protects protects nothing.

## Values in your own tables

A plugin that keeps a secret on a row of its own table, such as the
token of a mailbox, encrypts it the same way through
`sdk.EncrypterConsumer`:

```go
var _ sdk.EncrypterConsumer = (*Plugin)(nil)

// UseEncrypter keeps the encrypter AlphOne hands the plugin.
func (p *Plugin) UseEncrypter(encrypter sdk.Encrypter) {
	p.encrypter = encrypter
}
```

Encrypt before you store the row, and decrypt after you read it:

```go
stored, err := p.encrypter.Encrypt(ctx, "refresh_token", []byte(token))

token, err := p.encrypter.Decrypt(ctx, "refresh_token", stored)
```

The purpose names what the value is. Use one purpose for each kind of
value. `Decrypt` answers `sdk.ErrCannotDecrypt` for a value encrypted in
another workspace, by another plugin or for another purpose. Without a
key both answer `sdk.ErrNoEncryptionKey`.

## Why a copied value does not open

Every secret is encrypted together with where it belongs: the
workspace, the plugin, and the key or the purpose. This is called
associated data, and the value only opens with the same associated
data. [Google's Tink](https://developers.google.com/tink/aead) gives the
reason in plain words: tying a value to its user "prevents an attacker
from moving medical history from one user to another".

Here it means that someone who can write to the database cannot copy
one workspace's mailbox password into another workspace, or under
another key, and have it open there. The read answers
`sdk.ErrCannotDecrypt` instead.

## What other frameworks call it

| | The stored values | The encryption | Replacing the key |
| --- | --- | --- | --- |
| AlphOne | `sdk.SettingStore` | `sdk.Encrypter`, `ALPHONE_ENCRYPTION_KEY` | `ALPHONE_ENCRYPTION_PREVIOUS_KEYS` |
| WordPress | `wp_options`, `get_option` and `update_option`, shown under Settings | none, a password is stored in plain text | none |
| Laravel | packages such as spatie/laravel-settings | `Crypt` and the `Encrypter` contract, `APP_KEY` | `APP_PREVIOUS_KEYS` |
| Rails | packages such as rails-settings-cached | Active Record Encryption, `encrypts` | a list of keys, the last one encrypts |
| Django | packages such as django-constance | none in Django itself | `SECRET_KEY_FALLBACKS`, for signing only |

None of the others ties an encrypted value to where it belongs, so a
copied value opens anywhere their key does.
