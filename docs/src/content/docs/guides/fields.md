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

**Kind** says what the field holds. Seven kinds are available.

| Kind | Holds |
| ---- | ----- |
| Text | A short line, such as a job title |
| Long text | Several lines, such as a note |
| Number | A whole number, such as loyalty points |
| Yes or no | A checkbox |
| Date | A calendar day, written as `1990-04-17` |
| Choice | A short line, kept apart from Text so a later release can add a fixed option list |
| Repeater | A list of entries that share the same parts, such as a contact history |

Save, and the field exists. Open any contact and it is there, waiting to be
filled in.

## Fill a field in

Open a contact. The **Fields** section sits beside the tasks, or under them
on a narrow screen, with one input per field you created. Type, then press
**Save fields**.

A Long text field is a box several lines tall. Press Enter to start a new
line. The text keeps its line breaks when you save.

A field you never fill in stays empty. AlphOne stores nothing for it and it
costs nothing.

## Keep a list with a repeater

Some details are a list, not one value. A contact history is the common case.
Every entry has a date and a comment, and one contact gathers many entries
over time.

To make one, choose **Repeater** as the kind. A **Sub fields** list appears.
Press **Add sub field** once for every part an entry holds, and give each part
a label and a kind. For a history, that is `Date` as a Date and `Comment` as a
Long text.

A sub field can be any kind except Repeater, so a repeater never holds another
repeater. You do not type a name for a sub field. AlphOne makes one from its
label, so `Follow-up comment` becomes `followUpComment`. Two sub fields with
the same label get two names, such as `note` and `note2`.

The sub fields cannot be changed once the repeater exists. The reason is the
same as for the kind: old entries would no longer fit.

On a contact, a repeater lists its entries one under the other.

- Press **Add an entry to History**, with your repeater's label in place of
  History, to add an entry at the end.
- Use the arrows beside an entry to move it up or down.
- Press **Remove entry** to take one away.

Then press **Save fields**. Saving stores the whole list, in the order you
see it. An entry you leave completely empty is dropped when you save.

Saving replaces the list that was stored before. If two people change the
same list on two open pages, the second save wins and the first person's
changes are lost.

## Fill a field from a spreadsheet

You do not have to type every value in by hand. When you import a CSV or an
Excel file, your fields sit in the mapping dropdown beside Name, Email and
Phone. Point a column at one and the values arrive with the contacts.

The kind is checked before anything is stored. A row whose cell does not fit
its field fails, the reason names the field and its kind, and no contact is
created for that row. Fix the spreadsheet and import it again.

An empty cell stores nothing. The contact is created and the field stays
waiting, exactly as if you had never touched it.

A repeater is not in the mapping dropdown. A spreadsheet holds one value per
cell, and a repeater holds a whole list, so no column can fill it. Add the
entries on the contact after the import.

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
values come back. A repeater also needs the same sub fields, in the same
order and with the same names and kinds, or AlphOne refuses it.

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

A field belongs to the workspace that created it. Callers in another workspace
never see it, not in the API and not in introspection.

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

A repeater reads and writes as a list of entries. Each entry is an object
keyed by sub field name, and the API types the whole list as `JSON`:

```graphql
mutation {
  writeContactFields(
    contactId: "0198c000-0000-7000-8000-000000000401"
    values: {
      history: [
        { date: "2026-09-01", comment: "First call about the yearly plan." }
        { date: "2026-09-10", comment: "Sent the offer." }
      ]
    }
  )
}
```

Sending a repeater replaces its whole list. An empty list or `null` clears
it. Every cell is checked against its sub field's kind, and a refusal names
the cell by its place in the list, counting from 0, such as
`history[0].date expects DATE`. A key that no sub field holds is refused the
same way, such as `history[1].mood`.

To define a repeater from the API, pass its sub fields to `defineField`:

```graphql
mutation {
  defineField(
    name: "history"
    label: "History"
    kind: REPEATER
    subFields: [
      { name: "date", label: "Date", kind: DATE }
      { name: "comment", label: "Comment", kind: LONGTEXT }
    ]
  ) {
    id
    subFields {
      name
    }
  }
}
```

The API does not make sub field names for you. Send a camelCase name for each
one, unique inside the repeater.

## What stays fixed

A contact's **name** is not a field you can archive or rename away. It is what
AlphOne shows in lists, in tasks, and in search results, so it always exists.

Channels stay separate too. A phone number or an email address is an identity,
not a field, because AlphOne uses those to spot duplicate contacts. See
[Contacts](/guides/contacts/) for how that works.
