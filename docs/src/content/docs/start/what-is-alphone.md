---
title: What is AlphOne?
description: A plugin-first CRM that keeps customer conversations, contacts, and the people who answer them in one self-hosted place.
---

AlphOne is a plugin-first CRM. Customer conversations arrive through
channel plugins, land in a shared inbox, and stay attached to the contact
they belong to. The first channel plugin connects the WhatsApp Cloud API,
so messages sent to your WhatsApp business number appear live in the CRM
and can be answered from it.

## How it is built

- **One binary.** The Go backend serves the JSON API and the built React
  frontend from a single process. In production that is one container
  image plus a PostgreSQL database, nothing else.
- **Plugins compile in.** A plugin owns its routes, its database schema,
  and its frontend screens. The WhatsApp plugin is the reference
  implementation.
- **Boring, auditable auth.** Login, sessions, user administration, and
  login rate limiting come from the
  [Gopherium authentication bricks](https://docs.gopherium.org/authentication/overview/),
  extracted from this codebase and maintained as standalone libraries.

## Where to go next

- Run it on your machine: [Local development](/start/local-development/).
- Run it on your server: [Install](/self-hosting/install/).
- Connect your WhatsApp number: [Meta setup](/whatsapp/meta-setup/).

## License

AlphOne is source-available under a split license: the backend under the
Elastic License 2.0, the frontend under the GNU AGPL v3.0 or later. See
the [repository README](https://github.com/gopherium/AlphOne#license) for
the exact terms.
