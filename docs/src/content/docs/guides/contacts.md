---
title: Contacts
description: How AlphOne creates contacts from conversations, and how to search, rename, and add them.
---

Contacts are the people behind your conversations. AlphOne keeps one
contact per person, no matter how many channels they reach you on, and
the **Contacts** entry in the menu is where you manage them.

## Where contacts come from

Most contacts create themselves. The first time someone messages your
WhatsApp number, AlphOne creates a contact named after their WhatsApp
profile name and links their phone number to it as an identity.

When a person hides their profile name, the contact is named after the
raw phone number instead. Those number-named contacts are expected, and
fixing them is the point of the next section.

You can also add a contact by hand with the **New contact** button, for
people you want in the CRM before they ever message you.

## The contact list

The list shows one row per contact: its initials, its name with its first
identity under it, a badge for every channel it can be reached on, and the
date it was created. Click a name to open the contact. On a phone the rows
stack instead, and a tap on a row opens it.

The list opens with the newest contacts first. To sort another way, open
the menu on the **Name** or **Created** column heading and pick an order,
or choose **Sort by** under the gear button.

The list shows one page at a time. Move between pages at the bottom of the
list, and choose how many contacts a page holds under the gear button.
The search, the filters, the sort and the page stay in the address, so the
browser's Back button brings you back to the same place.

## Renaming

Open the actions menu at the end of a contact's row and choose
**Rename**. Type the new name and press **Rename**. You can also open the
contact and edit the name field there. The new name applies everywhere
immediately, including past conversations in the WhatsApp inbox, because
conversations reference the contact rather than copying its name.

Renaming is how you clean up number-named contacts: search for the
number, then rename the contact to the person's real name.

## Adding a task

The same actions menu offers **Add task**. It opens the New task form with
the contact already linked, so the task shows on the contact's page once
you create it.

## Searching and filtering

The search box on the contact list matches three things:

- the contact's **name**, by any part of it, ignoring case
- the **profile name** a channel reported for them
- their **phone number**, by any fragment, regardless of how you type
  it: searching `+34 612 34` finds a number stored as `34612…`

The filter button beside the search box narrows the list to the contacts
reached on the channels you pick, such as Email or WhatsApp.

## Identities

A contact's detail page lists its identities: the per-channel handles
that link conversations to the contact, such as a WhatsApp phone number.
Identities are created automatically by channels and cannot be edited.

Each line shows the channel and then the handle, such as
`Email: maria@example.com`. To remove an identity, click the trash icon
at the end of its line.

## The contact page

A contact's page has two columns. The left one holds the name, the
identities and the tasks, which is where the work happens. The right one
holds the date the contact was created and the custom fields. On a narrow
screen the right column moves under the tasks.

## Storing more about a contact

A contact holds only a name and its identities out of the box. To store an
address, a birth date, or anything else your business needs, create your own
fields. See [Fields](/guides/fields/).

## Deleting

Contacts cannot be deleted yet. A contact anchors its whole conversation
history, so deletion needs more care than a button. It is planned
deliberately rather than omitted.
