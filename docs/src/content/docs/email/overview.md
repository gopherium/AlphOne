---
title: Stored mail
description: What the email plugin keeps of a mail, how it threads it, how long it keeps it, what its event carries, and how to read it back.
---

The email plugin keeps the mail a workspace exchanges with its contacts.
Each stored mail sits on the contacts it names, in a thread, with its
attachments.

AlphOne does not connect to a mailbox. Mail reaches it in two ways:

- `alphone email:ingest` feeds one raw mail from a file into a
  workspace, see [Commands](/self-hosting/commands/#emailingest).
- `alphone seed -yes` adds a sample mailbox with a few threads to the
  demo data.

Every member of the workspace can read the stored mail back through the
API and download its files, see
[Reading mail through the API](#reading-mail-through-the-api).

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
- An attachment keeps its media type without the parameters its mail
  gives it. So `contentType` answers `application/pdf` for a file sent as
  `application/pdf; name="quote.pdf"`.
- An attachment with no media type is kept as `application/octet-stream`,
  under the file name its mail gives it. So is an attachment whose type
  does not read, names no subtype or is longer than 255 bytes.

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
- A copy that brings a provider thread id the stored mail lacks adds
  the stored mail's thread to that id, so a later mail with that id
  received near it can join it, see [Threads](#threads). A thread that
  already took such a mail stays apart until a later mail with that id,
  received near both threads, joins them when they share a contact.
- Reply ids and a Thread-Index that a copy brings, when no thread holds
  them yet, lead later mail to the stored mail's thread too.
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
   takes from `-provider-thread`. One id can lead to several threads. A
   mail joins one only when it was received between
   `ALPHONE_EMAIL_THREAD_WINDOW` before that thread's first mail
   and the same time after its last mail, and it joins the nearest. A
   mail that falls that close to two threads of its id joins them into
   one when they share a contact.
2. The reply headers, In-Reply-To and References.
3. The Thread-Index header that some mail programs add.
4. The subject, only for a reply or a forward such as `Re: Invoice`,
   only within `ALPHONE_EMAIL_THREAD_WINDOW` of the thread's
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
Mail that only a provider thread id ties together ends in the same
threads whatever order it arrives in, when every copy of every mail
carries the id and one contact is the sender's own on each copy. Mail
from more than one sender can group in another way when it arrives in
another order.

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

## Reading mail through the API

Every member of a workspace can read all of its stored mail through the
[GraphQL API](/reference/graphql-api/). A session works, and so does an
API token that holds the `email` area. This token reads mail and nothing
else:

```sh
alphone token:create -email you@example.com -name "mail reader" \
  -scope email:read
```

Four fields read the mail:

| Field | What it answers |
| --- | --- |
| `emailMailbox` | The workspace's mailbox, with its address and its aliases. It answers `null` when the workspace holds no mailbox yet. |
| `emailThreads` | One page of the inbox. The thread with the newest mail comes first. |
| `emailThread` | One thread by its id, with its mail. |
| `emailContactMessages` | One page of the mail linked to one contact, the newest first. |

A page holds `items`, `total` and `limit`, the way
[pages with a total](/reference/graphql-api/#pages-with-a-total) do.
`limit` follows the same range as `first`. `offset` skips that many rows.

This query reads the first page of the inbox:

```graphql
query Inbox {
  emailThreads(limit: 20) {
    total
    limit
    items {
      id
      subject
      lastMessageAt
      messageCount
      hasAttachments
      latest { from { name email } snippet }
    }
  }
}
```

Each thread shows its newest mail in `latest` and how many mails it holds
in `messageCount`. `hasAttachments` is true when one of its mails carries
an attachment. Inline images do not count.

### One thread and one mail

`messages` lists the mail of a thread, the newest first. It takes `limit`
and `offset` too.

```graphql
query Thread($id: UUID!) {
  emailThread(id: $id) {
    subject
    messages(limit: 10) {
      id
      direction
      from { name email }
      receivedAt
      snippet
    }
  }
}
```

`direction` is `inbound` for a mail that arrived and `outbound` for a
copy from the Sent folder. `from` is `null` for a mail that names no
sender.

To read the text of one mail, ask for it inside its thread:

```graphql
query Mail($thread: UUID!, $mail: UUID!) {
  emailThread(id: $thread) {
    message(id: $mail) {
      subject
      bodyState
      body { text html }
      attachments { filename contentType size state contentId downloadPath }
      originalPath
    }
  }
}
```

`emailThread` answers `null` for an unknown id and for a thread of
another workspace. `message` answers `null` for a mail of another thread.

A thread id can stop working. When a late mail joins two threads into
one, see [Threads](#threads), the thread that goes away answers `null`
from then on. Each mail keeps its own id. Read the inbox again to find
the thread that now holds it.

### The mail of one contact

`emailContactMessages` lists only the mail linked to that contact. A
thread can also hold mail that does not name the contact, and this list
leaves that mail out. Each item carries `threadId`, so you can open the
whole thread with `emailThread`. A contact of another workspace answers
an empty page.

```graphql
query ContactMail($contact: UUID!) {
  emailContactMessages(contactId: $contact, limit: 20) {
    total
    items { id threadId subject receivedAt snippet }
  }
}
```

`contactIds` on a mail names its contacts by id only. Reading the
contacts themselves needs the `contacts` area.

### What was kept of a mail

Some fields say what AlphOne kept of a mail.
[When a mail is too large or too broken](#when-a-mail-is-too-large-or-too-broken)
explains when each case happens. The original is the raw copy that
section speaks of.

| Field | Values |
| --- | --- |
| `bodyState` | `whole` when the text and the HTML are kept whole. `cut` when they were cut at the body cap. `headers_only` when AlphOne read only the headers. |
| `skippedParts` | How many parts AlphOne could not read and left out. The original holds them when it is kept. |
| `rawState` | `stored` when the original is kept as a file. `too_large` when it was over a size cap. `over_limit` when the workspace was full. `none` when the mail has no original to keep. |
| `state` of an attachment | `stored`, `too_large` or `over_limit`, with the same meaning as for the original. |
| `snippet` | A short preview. It is empty for an encrypted mail and for a mail read by its headers alone. |

A later version may add a state. To know whether a file downloads, read
`downloadPath` and `originalPath`. Each one is `null` when the file is
not kept.

### Downloading files

`downloadPath` names where an attachment downloads, and `originalPath`
names where the original mail downloads. Send a `GET` there with the
same session or token. A token needs `email:read`.

```sh
curl -OJ -H 'Authorization: Bearer a1_...' \
  'https://your-domain/api/plugins/email/attachments/<id>'
```

Replace `<id>` with the id of the attachment.

- An attachment downloads from `/api/plugins/email/attachments/{id}`.
- The original downloads from
  `/api/plugins/email/messages/{id}/original` as `message/rfc822`. It
  saves as `original-<id>.eml`, so its name never shows the subject.

A file never runs as a page in the browser:

- Every file comes with `Content-Security-Policy: sandbox` and
  `X-Content-Type-Options: nosniff`. So an HTML or SVG attachment cannot
  run a script.
- Every file comes with `Cache-Control: private, no-store`, so no cache
  keeps a copy.
- JPEG, PNG and WebP images, audio, and MP4 and 3GPP video show in the
  browser. Every other file downloads.
- The saved name is safe. AlphOne keeps only the part after the last `/`
  or `\`, drops control and invisible characters, and trims dots and
  spaces at both ends. A name with nothing left saves as `attachment`.
  `filename` in the API still answers the name the mail gave, so show it
  as plain text.

A download answers `404` when the file was not kept, when the id is
unknown, or when the file belongs to another workspace. An id that is not a UUID
answers `400`. A token without the `email` area answers `403`. Range
requests and conditional requests work, and the `ETag` is the file's
SHA-256.

### Remote images in the HTML

`html` in `body` is safe HTML. AlphOne removes scripts from it before it
stores the mail. It keeps remote images, such as
`<img src="https://example.com/logo.png">`.

A client that shows the HTML as it is loads those images. The sender can
then learn that the mail was opened, and from where. So block remote
images until the reader asks for them.

An image the mail carries itself points at `cid:` and an id. Find the
attachment whose `contentId` holds that id, and load the image from its
`downloadPath`. Percent-decode the id first, so `cid:lamp%2Bphoto@example.com`
matches the `contentId` `lamp+photo@example.com`.

### How many bodies one request reads

One request reads the `body` of at most
`ALPHONE_EMAIL_BODIES_PER_REQUEST` mails, 2 by default. A body can be
large, so this keeps one request from filling the server's memory.

A request that asks for more is rejected with the reason
`bodies_per_request_exceeded`, and `meta.max` names the bound. Ask for
one body at a time, through
`emailThread(id) { message(id) { body { text html } } }`. A list needs
no body, because each mail carries its `snippet`.

### How many people and file lists one request reads

One request reads at most `ALPHONE_EMAIL_LISTS_PER_REQUEST` lists, 50 by
default. Each `participants` and each `attachments` it asks for counts
one, so one mail asked for both counts two. `contactIds` does not count,
and `body` keeps its own bound above. One list can name hundreds of
people or files, so this keeps one request from filling the server's
memory.

A request that asks for more is rejected with the reason
`lists_per_request_exceeded`, and `meta.max` names the bound. A page of
25 mails with their people and their files asks for 50 lists, so it
fits. A page of 50, the default size, fits with one of the two. A list
of threads needs neither, because each mail carries its `from` and each
thread its `hasAttachments`.
