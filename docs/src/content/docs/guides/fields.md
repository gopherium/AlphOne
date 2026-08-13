---
title: Fields
description: Add your own contact fields from a screen, with no rebuild and no new release.
---

AlphOne ships with very little on a contact: a name, and the channels the
person reaches you on. Everything else is yours to add. The **Fields** entry
in the menu lets you create the fields your business actually uses, on a
running AlphOne, without a restart or a new version.

## Add a field

Open **Fields** and fill in three things.

**Label** is the text people see on screen, such as `Birth date`. Change it
whenever you like.

**Name** is what the API calls the field, such as `birthDate`. It starts with
a lowercase letter and holds only letters and digits. Pick it carefully,
because it cannot be changed later.

**Kind** says what the field holds. Six kinds are available.

| Kind | Holds |
| ---- | ----- |
| Text | A short line, such as a job title |
| Long text | Several lines, such as a note |
| Number | A whole number, such as loyalty points |
| Yes or no | A checkbox |
| Date | A calendar day, written as `1990-04-17` |
| Choice | A short line, kept apart from Text so a later release can add a fixed option list |

Save, and the field exists. Open any contact and it is there, waiting to be
filled in.

## Fill a field in

Open a contact. Below the tasks you will find a **Fields** section with one
input per field you created. Type, then press **Save fields**.

A field you never fill in stays empty. AlphOne stores nothing for it and it
costs nothing.

## Fill a field from a spreadsheet

You do not have to type every value in by hand. When you import a CSV or an
Excel file, your fields sit in the mapping dropdown beside Name, Email and
Phone. Point a column at one and the values arrive with the contacts.

The kind is checked before anything is stored. A row whose cell does not fit
its field fails, the reason names the field and its kind, and no contact is
created for that row. Fix the spreadsheet and import it again.

An empty cell stores nothing. The contact is created and the field stays
waiting, exactly as if you had never touched it.

One thing an import will not do is change a contact you already have. A row
matching an existing contact is skipped, and its fields are left alone. That
keeps an import from quietly overwriting work.

## The kind is checked when you save

AlphOne refuses a value that does not match the kind. A `Date` field will not
accept `not a date`, and a `Number` field will not accept `4.5`, because whole
numbers are what it holds. You see the reason on screen and nothing is stored.

This is why the kind cannot be changed after a field exists. Changing it would
leave old values that no longer fit.

## Archive a field you no longer need

Press **Archive** beside a field. It disappears from the contact screen and
from the API straight away.

Archiving does not delete anything. The values stay in the database. If you
create the field again later, with the same name and the same kind, the old
values come back.

## Using your fields from the API

A field you create becomes a real field on `Contact` in the GraphQL API, under
the name you chose. So after adding `birthDate` you can ask for it directly:

```graphql
query {
  contact(id: "0198c000-0000-7000-8000-000000000401") {
    name
    birthDate
  }
}
```

No rebuild, no code change. The field appears in schema introspection too, so
API tools and AI agents discover it on their own.

Writing values goes through one mutation:

```graphql
mutation {
  writeContactFields(
    contactId: "0198c000-0000-7000-8000-000000000401"
    values: { birthDate: "1990-04-17" }
  )
}
```

Send only the fields you want to change. A field you leave out keeps the value
it already holds, a field you send replaces it, and a field you send as `null`
is cleared.

A `Number` field holds a whole number between -2147483648 and 2147483647.
Anything outside that is refused, because the API answers it as an `Int`.

## What stays fixed

A contact's **name** is not a field you can archive or rename away. It is what
AlphOne shows in lists, in tasks, and in search results, so it always exists.

Channels stay separate too. A phone number or an email address is an identity,
not a field, because AlphOne uses those to spot duplicate contacts. See
[Contacts](/guides/contacts/) for how that works.
