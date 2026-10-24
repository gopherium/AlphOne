---
title: Contacts
description: Find the contacts behind the addresses your plugin meets, one or many at a time, and follow the addresses contacts gain.
---

A channel plugin, such as WhatsApp or a mail plugin, meets people by their
address: a phone number, a WhatsApp number or an email address. AlphOne keeps
one contact per person and links each of their addresses to it. The SDK calls
a linked address an **identity**. It is made of a channel, such as `email`,
and an identifier, such as `maria.perez@example.com`.

Your plugin gets four helpers for contacts in `sdk.Deps`. Keep the ones you
need from `Register`:

| Field | What it does |
| --- | --- |
| `deps.Resolver` | Finds the contact behind one address, and creates one when no contact has it. |
| `deps.Contacts` | Looks one address up without creating anything, and creates a contact that owns several addresses. |
| `deps.Matcher` | Finds the contacts behind many addresses in one call. |
| `deps.Identities` | Lists the addresses contacts gain, in a fixed order. |

## Every call stays in one workspace

Every helper works in the workspace `ctx` carries, and never sees another
workspace's contacts. AlphOne puts the workspace on `ctx` for every signed in
request. Work your plugin starts on its own, such as a background sweep, sets
it with `sdk.WithTenant`, the way the WhatsApp media fetcher does. A `ctx` that
carries no workspace works in the default one.

## How addresses are compared

Every helper puts an address in its channel's canonical form before it
compares or stores it:

- The channel is trimmed and lower-cased, so `Email` is `email`.
- An email address is trimmed and lower-cased. `Maria.Perez@Example.com` is
  the same address as `maria.perez@example.com`. Nothing else is folded, so
  `maria.perez+news@example.com` and `mariaperez@example.com` are other
  addresses.
- A phone number keeps only its digits, with a leading `+` when it starts
  with one. `+184 467 235` becomes `+184467235`.
- On any other channel, such as `whatsapp`, the identifier is trimmed and
  then compared exactly.

## Find or create one contact

`deps.Resolver` is a `sdk.ContactResolver`. Its `Resolve` answers the contact
that owns an address. When no contact owns it, `Resolve` creates one and links
the address to it. The new contact is named after the display name you pass,
or after the address when the name is blank. Two calls racing on one new
address get the same contact. The WhatsApp plugin calls it for every message
that arrives, so each new sender becomes a contact:

```go
owner, err := p.resolver.Resolve(ctx, "whatsapp", m.sender, m.senderName)
if err != nil {
	return err
}
```

`deps.Contacts` is a `sdk.ContactDirectory`, for when an unknown address must
not become a contact on its own. `FindByIdentity` answers the contact that
owns one address, and whether one does. `CreateWithIdentities` creates a
contact that owns several addresses at once, as the importer does for each
row. When another contact already owns one of the addresses, it creates
nothing, answers that contact and reports `false`.

## Match many addresses at once

`deps.Matcher` is a `sdk.ContactMatcher`. Its `MatchIdentities` takes a list
of addresses on one channel and answers, in one database read, the contact
behind each address a contact owns. It never creates a contact, and an address
no contact owns is left out of the answer. Use it where one message names many
people, such as the sender and every recipient of a mail.

The answer is a map keyed by each address in its canonical form, so look an
address up in that form:

```go
owners, err := p.matcher.MatchIdentities(ctx, "email", []string{
	"Maria.Perez@Example.com",
	"ada.lovelace@example.com",
})
if err != nil {
	return err
}
owner, found := owners["maria.perez@example.com"]
```

One call takes at most `deps.ContactMatchMax` addresses. The server sets it
with `ALPHONE_CONTACT_MATCH_MAX`, 100 when unset, see
[Configuration](/self-hosting/configuration/#contacts). Every address in the
list counts, a repeated one too. A longer list is refused whole with
`sdk.ErrTooManyIdentifiers`, and nothing is matched, so split a long list into
batches of `deps.ContactMatchMax`.

One address that is empty in its canonical form, such as a blank one or a
phone number holding no digits, also refuses the whole list, with
`sdk.ErrInvalidContact`. Drop such addresses before the call.

## Follow the addresses contacts gain

`deps.Identities` is a `sdk.IdentityFeed`. It lists every address the
workspace's contacts gain on one channel, however it was added: with a new
contact, on a contact's page, by an import or by a plugin. Use it when your
plugin keeps data only for known contacts and must catch up when a contact
appears or gains an address. A mail plugin, for example, fetches the recent
mail of each new address.

Your plugin keeps a `sdk.IdentityCursor`, which marks how far it has read.
Each call answers the next page and the cursor after it. This loop asks for
pages of `deps.ContactMatchMax`, kept in `p.matchMax`, so each page fits one
match:

```go
for {
	added, next, err := p.identities.IdentitiesAddedSince(
		ctx, "email", cursor, p.matchMax,
	)
	if err != nil {
		return err
	}
	if len(added) == 0 {
		return nil
	}
	for _, identity := range added {
		err = p.fetchRecentMail(ctx, identity)
		if err != nil {
			return err
		}
	}
	cursor = next
	err = p.saveCursor(ctx, cursor)
	if err != nil {
		return err
	}
}
```

Each `sdk.AddedIdentity` holds the `ContactID` that gained the address, the
`Identifier` in its canonical form, and `AddedAt`, the time AlphOne saved it.

A few rules keep a reader correct:

- A page holds at most the `limit` you pass, and never more than
  `deps.ContactMatchMax`, so a page always fits one call to the matcher.
  A page can hold fewer than you asked while more wait, so read until a page
  comes back empty. An empty page answers the cursor you passed. A `limit`
  below one is refused.
- Save the cursor only after you have handled its page. A plugin that stops
  halfway starts again from its saved cursor and handles part of that page a
  second time, so handling an address twice must do no harm.
- Keep one cursor for each workspace and channel you follow.
- The zero `sdk.IdentityCursor{}` is the place before every address. A reader
  starting there lists every address the workspace's contacts hold today, and
  then each new one. `Horizon` answers the place a new reader starts from to
  see only what comes next: every address whose save had not finished when
  you asked, and every later one. Addresses saved since the oldest write
  still running on the PostgreSQL server began can show up too, so a long
  open transaction can make that a long list.
- A saved cursor can name a transaction the PostgreSQL server has not
  reached, after AlphOne's database is restored onto another server. The
  feed then refuses it with `sdk.ErrCursorAhead`. Start again from the zero
  cursor and handle every address once more.
- The feed reports only additions. A removed address is not reported, and one
  removed before you read it never shows up.
- Follow the cursor, never `AddedAt`. AlphOne stamps that time as it saves,
  and a save can finish after a later one.

## Why the feed reads by transaction

PostgreSQL numbers every transaction that writes, counting up, and the feed
orders addresses by the number of the transaction that saved them. It answers
only addresses whose transaction number is lower than that of every
transaction still running. So a save that started first but finishes last is
never skipped: the feed holds every later address back until it finishes. The
price is that a long open transaction makes the feed wait rather than skip.
Any open transaction that writes counts, in any database on the same
PostgreSQL server, not only AlphOne's, and the feed answers nothing past it
until it ends.

Transaction numbers belong to one PostgreSQL server. A dump restored onto
another server, such as a new PostgreSQL major version, keeps the numbers
the old server gave, while the new server counts from the start again. Each
time AlphOne starts, it marks every address numbered past the server's count
as saved by that start, so a reader from the zero cursor sees it at once. A
saved cursor past the count is refused, as the rules above say.

## When a call is refused

| Error | Cause |
| --- | --- |
| `sdk.ErrInvalidContact` | The channel is empty, an address is empty in its canonical form, or a new contact's name is blank |
| `sdk.ErrTooManyIdentifiers` | A match asked for more addresses than `deps.ContactMatchMax` |
| `sdk.ErrCursorAhead` | A feed cursor names a transaction the PostgreSQL server has not reached |

Test each with `errors.Is`. A feed page asked for fewer than one address is
refused too. Any other error comes from the database.
