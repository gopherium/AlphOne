---
title: Email
description: Where the Email row shows, and how the inbox lists the workspace's mail.
---

AlphOne does not connect to a mailbox yet. Mail reaches a workspace in
two ways:

- `alphone email:ingest` feeds one raw mail from a file, see
  [Commands](/self-hosting/commands/#emailingest).
- `alphone seed -yes` adds a sample mailbox with a few threads to the
  demo data.

[Stored mail](/email/overview/) says what AlphOne keeps of each mail and
how it groups mail into threads. This page shows where you read it.

## The Email row

The **Email** row in the menu opens the inbox. In Spanish the row reads
**Correo electrónico**. It shows only once the workspace holds a
mailbox. A fresh install holds none, so its menu has no Email row. The
first `email:ingest` run with `-mailbox` and `-yes` stores the mailbox,
and so does the demo data.

The menu checks for a mailbox once as the admin opens and again after
each sign-in. After the first mail reaches a new workspace, reload the
admin or sign in again to see the row. Every member of the workspace
sees it.

If the check takes too long, the row shows anyway, see
[Plugin hooks](/self-hosting/configuration/#plugin-hooks). The inbox
then says **No mailbox is connected** until a mailbox is stored. The
same words show to anyone who types the inbox's address, `/email`, in a
workspace with no mailbox.

## The inbox

The inbox lists the workspace's threads, the one with the newest mail
first. Under the title it names the mailbox it reads, such as "Read the
mail of `info@example.com` in one place."

Each row is one thread, and it shows its newest mail:

- **From**: the sender's initials, name and address. When the workspace
  sent the newest mail itself, a **Sent** badge follows the name.
- **Subject**: the thread's subject, and under it the first words of the
  newest mail. An **Encrypted** or **Signed** badge follows the subject
  when the newest mail carries one, and a file icon when the thread holds
  attachments.
- **Messages**: how many mails the thread holds.
- **Date**: when the newest mail arrived.

AlphOne cannot read the text of an encrypted mail, so when the newest
mail is encrypted the row shows **Encrypted message** in place of the
first words. When AlphOne could read only the headers of the newest
mail, the row shows **No text could be read**. A **Signed** badge says
the mail carries a signature. AlphOne does not check it.

The list shows one page at a time. Move between pages at the bottom of
the list, and choose how many threads a page holds under the gear
button. The page stays in the address, so the browser's Back button
brings you back to the same place. The inbox has no search, filter or
sort yet, and a row does not open its thread yet.

On a narrow screen the list keeps the **From** column alone, with the
subject and its badges under the sender's name.

A workspace that holds a mailbox but no mail yet shows **You're set up.
No mail yet.**

## In Spanish

A reader who picked Spanish for the interface sees these words in place
of the English ones on this page.

| English | Spanish |
| --- | --- |
| Email | Correo electrónico |
| No mailbox is connected | No hay ningún buzón conectado |
| Read the mail of `info@example.com` in one place. | Lee el correo de `info@example.com` en un solo lugar. |
| From | De |
| Sent | Enviado |
| Subject | Asunto |
| Encrypted | Cifrado |
| Signed | Firmado |
| Messages | Mensajes |
| Date | Fecha |
| Encrypted message | Mensaje cifrado |
| No text could be read | No se ha podido leer el texto |
| You're set up. No mail yet. | Todo listo. Todavía no hay correo. |
