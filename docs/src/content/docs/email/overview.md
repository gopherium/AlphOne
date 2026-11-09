---
title: Stored mail
description: What the email plugin keeps of a mail, how it threads it, how long it keeps it, and what its event carries.
---

The email plugin keeps the mail a workspace exchanges with its contacts.
Each stored mail sits on the contacts it names, in a thread, with its
attachments.

AlphOne does not connect to a mailbox. Mail reaches it in two ways:

- `alphone email:ingest` feeds one raw mail from a file into a
  workspace, see [Commands](/self-hosting/commands/#emailingest).
- `alphone seed -yes` adds a sample mailbox with a few threads to the
  demo data.

Each workspace holds one mailbox, which is an address and any aliases
that also reach it, such as `info@example.com` and `sales@example.com`.
Storing a mail changes nothing in the mailbox it came from. Every limit
on this page is a setting, see
[Configuration](/self-hosting/configuration/#email-plugin).

## What is stored

Only mail that names a known contact is stored. A known contact is a
contact that holds the address as an email, see
[Contacts](/guides/contacts/). AlphOne reads the addresses in From,
Reply-To, To and Cc, leaves out the mailbox's own address and its
aliases, and looks for the contact behind each one. Upper and lower case
do not matter.

- A mail that names no known contact is not stored. It leaves no row,
  no file and no trace of its addresses.
- AlphOne never creates a contact from a mail.
- A mail that names a known contact is stored whole, with every other
  person on it. So a stranger who copies a known contact is stored too,
  on that contact.
- A mail is linked to every known contact it names, and shows for each
  of them.
- A Bcc field is read only on a copy from the Sent folder. On a mail
  that arrives it is neither matched nor kept, because the sender can
  write anything there.

Each stored mail keeps:

- its text and a safe copy of its HTML, in PostgreSQL
- the names and addresses of the people on it
- the original mail as one file, exact to the byte, called the raw copy
- each attachment and inline image as a file of its own

The files go where [Files](/self-hosting/configuration/#files) says.
Attachments are not scanned for viruses.

## When a mail is too large or too broken

AlphOne keeps what it can of a mail and never drops one that names a
known contact.

- A body over `ALPHONE_EMAIL_BODY_MAX_BYTES` is cut on the row, where a
  letter starts. The raw copy keeps the whole mail when it fits.
- A mail over `ALPHONE_EMAIL_MESSAGE_MAX_BYTES`, or too broken to read,
  is read by its headers alone. When they name a known contact, the mail
  keeps a row with its headers only. A broken mail also keeps its raw
  copy, and a mail over that size keeps none.
- When those headers name no known contact, nothing is stored, and the
  server log gets one line with the mailbox's id and a fixed reason.
- A part AlphOne cannot read is left out, and the rest of the mail is
  stored. The raw copy keeps that part.
- An attachment over `ALPHONE_EMAIL_ATTACHMENT_MAX_BYTES` or over
  `ALPHONE_STORAGE_MAX_BYTES` keeps its name with no file.
- A raw copy over `ALPHONE_STORAGE_MAX_BYTES` is not kept, and the text
  and the attachments of that mail still are.
- An attachment with no media type is kept as `application/octet-stream`,
  under the file name its mail gives it. So is an attachment whose type
  the file store refuses, such as a type that names no subtype.

## The workspace limit

Each workspace keeps at most `ALPHONE_EMAIL_WORKSPACE_MAX_BYTES` of
mail, 5 GiB by default. The limit counts the files of each mail and the
text it keeps: its text, its HTML, its preview and the list of people on
it.

The limit stops keeping files, never text. A mail that would take the
workspace past it is still stored with its text, its people and its
thread. Its raw copy and its attachments are not kept as files, and
each attachment keeps its name. A later mail that fits keeps its files
again.

Each mail is checked against the mail already stored and the files
still being saved, so mails stored at the same moment can together keep
files past the limit by up to their own size. The next mail sees the
real total.

## Copies of one mail

The same mail is stored once, whether it is fed twice or comes once
from the inbox and once from the Sent folder.

- A second copy that names another known contact links that contact to
  the stored mail and adds the new addresses to it. It stores no second
  mail.
- A copy that brings a provider thread id the stored mail lacks leads
  later mail with that id to the stored mail's thread.
- A copy from the Sent folder marks the mail as sent by the workspace.
  A mail that only writes the mailbox's own address in From stays a
  mail that arrived, because anyone can write that address there.
- Two different mails that share a Message-ID are both kept.
- A mail cut at the body cap, read by its headers alone, or holding a
  part AlphOne cannot read counts as one mail only when its copies
  match byte for byte.

## Threads

AlphOne puts each mail in a thread. It looks for one in this order:

1. The thread id the mail's provider gives it, which `email:ingest`
   takes from `-provider-thread`. A mail with no reply headers and no
   Thread-Index counts only within `ALPHONE_EMAIL_SUBJECT_THREAD_WINDOW`
   of the thread's first or last mail. Once the thread the id leads to
   has taken no mail for that long, a later mail with the id moves the
   id to the thread that mail joins.
2. The reply headers, In-Reply-To and References.
3. The Thread-Index header that some mail programs add.
4. The subject, only for a reply or a forward such as `Re: Invoice`,
   only within `ALPHONE_EMAIL_SUBJECT_THREAD_WINDOW` of the thread's
   first or last mail, 14 days by default, and never for automated
   mail. Upper and lower case do not matter.

A mail that finds no thread starts a new one.

A thread takes a mail only when they share a contact. For the provider's
thread id, the Thread-Index and the subject, that contact must be the
sender's own: the contact on From for a mail that arrives, or a
recipient's contact for a copy from the Sent folder. For a reply header,
any contact the mail names counts. So two customers who both write
`Invoice` never share a thread, even with the same person copied on
both, and a mail that points at the threads of two customers merges
nothing.

A reply stored before its parent still joins it. A parent that arrives
late and connects two threads of the same contact joins them into one.

A thread lists its mail by the time the mailbox received each one,
never by the Date header alone, which the sender writes. `email:ingest`
takes that time from `-received`, or the time it runs. A provider that
took over a mailbox from another one can give the time of that move, or
the Date header, as the received time of the older mail.

Nothing checks who sent a mail. A forged From passes every rule above,
so a forged mail can enter a thread.

## How long mail is kept

A stored mail stays, with every name it was stored with. AlphOne has no
way to delete a stored mail, and it deletes no mail because a person
named on it asks.

AlphOne has no way to delete a contact either, and a contact who holds
stored mail cannot be deleted.

A background sweep removes only the files that no stored mail holds,
such as the files of a mail that failed to store, or of a run that
stopped halfway. It runs every `ALPHONE_EMAIL_SWEEP_INTERVAL`, one hour
by default.

## When a mail cannot be stored

One mail never holds up the rest. When a mail fails to store, its files
are queued for removal at once. When the database refuses it for good,
or it keeps failing for longer than `ALPHONE_EMAIL_RETRY_WINDOW`, six
hours by default, it is set aside. AlphOne then keeps only its reference
and a fixed reason, never its addresses, its subject or its text, and
answers `failed` for it from then on.

The plugin's log lines name a workspace, a mailbox or a stored mail by
its id. They never hold an address, a subject or the text of a mail.

## The event

A new mail from a person publishes `email.message.received`, which a
[webhook](/reference/webhooks/) can subscribe to. Its `data` carries:

| Key | Value |
| --- | --- |
| `id` | The id of the stored mail. |
| `thread_id` | The id of its thread. |
| `contact_ids` | The ids of every contact the mail is linked to. |
| `subject` | The subject as the mail keeps it. |
| `text` | The text of the mail, cut where a letter starts at `ALPHONE_EMAIL_EVENT_TEXT_MAX_BYTES`, 16 KiB by default. It is empty for an encrypted mail. |

The event never carries HTML or attachments. It does not fire for:

- automated mail, such as an auto reply, a bounce or list mail
- a copy from the Sent folder
- a mail whose From is the mailbox's own address or an alias
- a mail stored with its headers only
- a second copy of a stored mail
- the sample mail that `seed` adds

AlphOne judges a mail automated by headers the sender writes, such as
`Auto-Submitted`, `Precedence` and `List-Id`. So a person who writes
through a mailing list fires no event, and any sender can add
`Precedence: bulk` to fire none.

Each webhook delivery keeps its payload in the database, so a copy of
the subject and of up to 16 KiB of the text stays there. Once the
delivery was sent or failed for good, AlphOne clears that copy after
`ALPHONE_WEBHOOK_PAYLOAD_AGE`, 30 days by default. A delivery still
waiting to be sent keeps it however long it waits. Revoking the webhook
removes those copies at once, see
[How long a payload is kept](/reference/webhooks/#how-long-a-payload-is-kept).
