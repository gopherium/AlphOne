---
title: Fields
description: Add your own contact fields from a screen, with no rebuild and no new release.
---

AlphOne ships with very little on a contact: a name, and the channels the
person reaches you on. Everything else is yours to add. The **Fields** entry
in the menu lets you create the fields your business actually uses, on a
running AlphOne, without a restart or a new version.

## Add a field

Open **Fields** and fill in two things.

**Label** is the text people see on screen, such as `Birth date`.

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

Press **Add field**, and the field exists. Open any contact and it is there,
waiting to be filled in.

AlphOne makes the field's name from its label, so `Birth date` becomes
`birthDate`. The API uses that name, and the field list shows it under
**API name**. If another field already has that name, even an archived one,
the new name gets the next free number, such as `birthDate2`. The number goes
straight after the name, so `Address 2` becomes `address22` when `address2`
is taken. The name cannot be changed later.

If a field in the list already has the label, the screen says so and adds
nothing. Case and spaces at either end do not matter, so `birth date` counts
as `Birth date`.

A few names are reserved, such as `name`, `tasks` and `constructor`. A label
that makes one of them gets a number too, so `Name` becomes `name2`. A label
with no Latin letters or digits, such as `???` or one written in Cyrillic, is
named after the word `field`. That name is reserved too, so it becomes
`field2`.

## Fill a field in

Open a contact. The **Fields** section sits beside the tasks, or under them
on a narrow screen. Type into a field, then press **Save fields**. The button
stays greyed out until you change a field.

When a contact has repeaters, they split the other fields into groups, and each
group gets its own **Save fields** button under it. A button turns on once you
change a field in its own group, and it saves every field you changed on that
contact, wherever it sits on the page.

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

Each sub field sits on one line, with its label and kind side by side. On a
narrow screen the line wraps. The arrows at the end of the line move it up or
down. The trash icon, named **Remove sub field**, takes it out of the list.

A sub field can be any kind except Repeater, so a repeater never holds another
repeater. AlphOne names a sub field from its label too, so
`Follow-up comment` becomes `followUpComment`. Two sub fields with the same
label get two names, such as `note` and `note2`. A sub field labelled `ID`
becomes `id2`, because each entry keeps its own id under `id`.

The sub fields cannot be changed once the repeater exists. The reason is the
same as for the kind: old entries would no longer fit.

On a contact, each repeater sits in its own box, with its label as the
heading. A form to add an entry sits on top. The entries follow, the last one
added first. Each date in the form starts on today's date.

- Fill in the form and press **Add an entry to History**, with your
  repeater's label in place of History. The button stays off until you fill
  in a part yourself. Spaces alone do not count, and neither does the date
  the form starts with.
- Press the pencil beside an entry's date to change the entry in place, then
  **Save entry** or **Cancel**. **Save entry** stays off while every part is
  blank.
- Press the trash icon, and the row asks **Remove this entry?** Press
  **Remove** to confirm or **Keep** to leave it.

Each add, save and removal is stored at once. **Save fields** never saves a
repeater, and a contact whose fields are all repeaters has no **Save fields**
button. Each change touches only its own entry. If two people edit the same
entry, the last save wins.

A list holds at most 500 entries by default. See
[Configuration](/self-hosting/configuration/#fields-plugin) to change the cap.

## Fill a field from a spreadsheet

You do not have to type every value in by hand. When you import a CSV or an
Excel file, your fields sit in the mapping dropdown beside Name, Email and
Phone. They come after those three, in the order set on the **Fields** screen.
Point a column at one and the values arrive with the contacts.

A field named `email` or `phone` stays out of the dropdown, because Email and
Phone already use those names. The labels `Email` and `Phone` make exactly
those names, so choose a longer label, such as `Email consent`.

The kind is checked before anything is stored. A row whose cell does not fit
its field fails, the Reason column names the field and its kind in your
language, and no contact is created for that row. Fix the spreadsheet and
import it again.

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
numbers are what it holds. You see the reason in the group whose
**Save fields** you pressed, and nothing is stored.

This is why the kind cannot be changed after a field exists. Changing it would
leave old values that no longer fit.

## Put the fields in order

The list on the **Fields** screen shows your fields in order. A new field goes
to the end. Until you move one, the fields stay in the order you added them.

Each row ends with three icons. The up arrow moves the field one place up, and
the down arrow moves it one place down. For a field labelled `Birth date` they
are named **Move Birth date up** and **Move Birth date down**. The up arrow is
off on the first row, and the down arrow is off on the last. The trash icon
asks before it archives the field, as the next section explains.

The list changes as soon as you press an arrow, and the new order is saved for
everyone in your workspace. If the save fails, the fields go back to where
they were and the screen says **The fields could not be ordered.** If someone
added or archived a field after you opened the screen, the move is refused
with **The field list just changed. The new order was not saved.** The list
then shows the fields as they are now, so move the field again.

The contact page shows your fields in this order, repeaters too. The mapping
dropdown of an import and `fields` in the API use the same order. A repeater
moves as a whole. Its sub fields keep the order you gave them when you made it.

## Archive a field you no longer need

Press the trash icon at the end of the field's row. For a field labelled
`Birth date` it is named **Archive Birth date**. Nothing is archived yet. The
row asks **Archive this field?** Press **Archive** to confirm, or **Keep** to
leave the field as it is. The question starts on **Keep**, so pressing Enter
by mistake keeps the field.

Once you confirm, the field disappears from the contact screen and from the
API. If the archive fails, the screen says so and the trash icon comes back,
so you can try again.

Archiving does not delete anything, even though the icon is a trash can. The
values stay in the database, and the archived field keeps its name. So a new
field with the same label gets a numbered name and starts empty. Only the API
can bring the old field back with its values, and it comes back at the end of
the list, like a new field. See
[Using your fields from the API](#using-your-fields-from-the-api).

## Using your fields from the API

A field you create becomes a real field on `Contact` in the GraphQL API, under
its API name. So after adding `Birth date` you can ask for `birthDate`
directly:

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

Writing values goes through `writeContactFields`:

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

`writeContactFields` refuses any value for a repeater, even `null`, with the
reason `field_repeater_entries_only`, and stores nothing from that request. A
repeater takes its entries one at a time, as shown below.

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

The API does not make names for you. `defineField` takes a `name` such as
`birthDate`. It starts with a lowercase letter and uses only a to z, A to Z
and 0 to 9. Each sub field needs such a name too, unique inside its repeater.
`id` is refused for a sub field, because each entry keeps its own id under
that name.

`reservedFieldNames` lists the names no field can take. They are the fields
`Contact` is built with, such as `name` and `tasks`, and the names every
JavaScript object has, such as `constructor` and `toString`. Such a name is
refused with `field_name_reserved`. Upgrading AlphOne moves a field stored
under one of the JavaScript names to the next free name, such as
`constructor2`, with its values.

`fields(includeArchived: true)` also lists archived fields, each with its
`archivedAt`, so you can find the name to send. Calling `defineField` with an
archived field's name brings that field back with its values and the label
you send. It goes to the end of the list, like a new field. The kind must
match, and for a repeater so must the sub field names and kinds, in the same
order. Otherwise the call is refused with `field_kind_locked`. The answer
carries the field's old id.

`fields` lists the fields in the order set on the **Fields** screen. With
`includeArchived: true`, the archived fields come after the others.

`orderFields` sets the order from the API. Send the id of every field that is
not archived, in the order you want:

```graphql
mutation {
  orderFields(
    ids: [
      "0198c000-0000-7000-8000-000000000302"
      "0198c000-0000-7000-8000-000000000301"
    ]
  )
}
```

It answers `true`. The list must name every field in your workspace that is
not archived, each one exactly once. A list that leaves a field out, names one
twice, or holds an id that is archived, unknown or from another workspace is
refused with `field_order_incomplete`, and the order stays as it was. That way
an order built from an old copy of the list is never saved over a field
someone added or archived since. Read `fields` again and send the whole list.

A repeater reads as a list of entries typed `JSON`, the last one added first.
Each entry is an object keyed by sub field name, with the `id` AlphOne gave
it. A repeater with no entries reads `null`. Ask for `history` the same way as
`birthDate`, and the answer looks like this:

```json
{
  "data": {
    "contact": {
      "history": [
        {
          "id": "0198c000-0000-7000-8000-000000000501",
          "date": "2026-09-10",
          "comment": "Sent the offer."
        }
      ]
    }
  }
}
```

`addContactFieldEntry` puts one entry at the top of the list:

```graphql
mutation {
  addContactFieldEntry(
    contactId: "0198c000-0000-7000-8000-000000000401"
    field: "history"
    entry: { date: "2026-09-10", comment: "Sent the offer." }
  )
}
```

It answers the stored entry with its new `id`. Every cell is checked against
its sub field's kind, and a refusal names the cell, such as
`history.date expects DATE`. A key that no sub field holds is refused and
named too, such as `history.mood`. So is `id`. An entry whose cells are all
blank is refused with `field_entry_empty`. An add to a full list is refused
with `field_entries_full`, and its `meta.max` names the cap.

`updateContactFieldEntry` changes the entry that `entryId` names:

```graphql
mutation {
  updateContactFieldEntry(
    contactId: "0198c000-0000-7000-8000-000000000401"
    field: "history"
    entryId: "0198c000-0000-7000-8000-000000000501"
    entry: { date: "2026-09-10", comment: "Sent the offer by email." }
  )
}
```

The entry you send replaces all its cells, so a cell you leave out is dropped.
The cells get the same checks as an add, but you may send the entry's own `id`
back. The entry keeps its id and its place, and the answer is the entry as
stored.

`deleteContactFieldEntry` takes the same `contactId`, `field` and `entryId`,
and answers `true`. An update or a removal naming an id the list does not
hold is refused with `field_entry_not_found`. See
[GraphQL API](/reference/graphql-api/#reasons) for where a refusal carries its
reason and `meta`.

## What stays fixed

A contact's **name** is not a field you can archive or rename away. It is what
AlphOne shows in lists, in tasks, and in search results, so it always exists.

Channels stay separate too. A phone number or an email address is an identity,
not a field, because AlphOne uses those to spot duplicate contacts. See
[Contacts](/guides/contacts/) for how that works.
