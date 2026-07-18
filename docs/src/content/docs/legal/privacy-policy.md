---
title: Privacy policy
description: What data an AlphOne deployment processes, where it lives, and how to have it deleted.
---

*Effective date: 2026-07-18*

This policy covers this **documentation site** (`docs.alph.one`) and
describes how an **AlphOne deployment** handles WhatsApp data. When a
Meta app lists this page as its privacy policy, the policy applies to
the AlphOne deployment connected to that app.

AlphOne is self-hosted software. If you are reading this because you
messaged a business that runs its own AlphOne instance, that business is
responsible for your data, not the AlphOne project. Self-hosters: see
[Your own deployment](#your-own-deployment) below.

## What we process, and why

When you send a WhatsApp message to a number connected to an AlphOne
deployment, Meta's WhatsApp Business Cloud API delivers it to that
deployment, which stores:

- your phone number and WhatsApp profile name
- the content of your messages, including attachments
- message timestamps and delivery metadata

This data is used for exactly one purpose: operating a customer
relationship tool, so the people running the deployment can read and
answer your conversation and keep a contact record for you.

## Where it lives

Everything is stored in the deployment's own PostgreSQL database, on
infrastructure the operator controls. There is no third-party analytics,
no advertising, no selling or sharing of data with anyone. The only
third party involved is Meta, which transmits WhatsApp messages under
[its own terms](https://www.whatsapp.com/legal/business-data-transfer-addendum).

Data is kept until the operator deletes it or a deletion is requested.
Routine encrypted backups of the database may exist and expire on a
short rotation.

## Security

Deployments are served exclusively over HTTPS. Access to conversations
requires an authenticated operator account, with rate-limited logins and
server-side sessions.

## Data deletion

To have your conversation history and contact record deleted from a
deployment using this policy, either:

- send a WhatsApp message to the same business number asking for your
  data to be deleted, or
- open an issue at
  [github.com/gopherium/AlphOne](https://github.com/gopherium/AlphOne/issues)
  (do not include your phone number in the public issue, just ask to be
  contacted).

Deletion removes your contact record, conversation, messages, and
attachments. This page also serves as the data deletion instructions for
the associated Meta apps.

## The documentation site

`docs.alph.one` is a static site. It sets no cookies, runs no analytics,
and collects no personal data. It is hosted on GitHub Pages, whose
servers may keep standard access logs under
[GitHub's privacy statement](https://docs.github.com/en/site-policy/privacy-policies/github-general-privacy-statement).

## Your own deployment

If you self-host AlphOne, you are the data controller for everything
your instance stores. You are welcome to adapt this text for your own
deployment: replace the deployment name, the deletion contact, and the
backup details with yours, host it at a URL you control, and use that
URL as the Privacy Policy and data deletion instructions in your Meta
app settings. Depending on where you and your customers are, additional
obligations (such as GDPR) may apply to you.

## Contact

Questions about this policy:
[github.com/gopherium/AlphOne](https://github.com/gopherium/AlphOne/issues).
