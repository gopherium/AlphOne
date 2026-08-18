---
title: Local development
description: Run the API, the database, and the React frontend on your machine, from clone to first login.
---

This page takes you from a fresh clone to a working login. You will run
three things: a PostgreSQL container, the Go backend, and the Vite dev
server for the frontend.

## Prerequisites

- **Go** 1.26 or newer
- **Node.js** 26 or newer, with **pnpm** 11 (`npm install -g pnpm`)
- **Docker** with the compose plugin

## 1. Clone and start the database

```sh
git clone https://github.com/gopherium/AlphOne.git
cd AlphOne
make db-up
```

`make db-up` starts a PostgreSQL 18 container on port **5433** (not the
default 5432, so it never collides with a local PostgreSQL). The
superuser is `postgres` with password `alphone`. The same container also
backs the Go test suite.

## 2. Configure the environment

Copy the template:

```sh
cp .env.example .env
```

The binary loads `.env` from the working directory at startup, and real
environment variables take precedence over it. The template's
`ALPHONE_DATABASE_URL` already points at the container from step 1, so
there is nothing to edit for a first run.

## 3. Create the first login

The quickest start is the development seeder. One command migrates the
database and fills it with a demo data set:

```sh
make seed
```

It creates two logins on the same password `password1234`. One is an
admin, `admin@example.com`, and one is a member, `maria@example.com`,
so you can see both tiers without making a second account yourself.
It also creates a few contacts, a day of tasks, and WhatsApp
conversations with text, an image, and delivery ticks, so every screen
has something to show.
Running it again is safe, it repairs a half-seeded database instead of
duplicating anything. The credentials are public knowledge: never run
the seeder against a production database.

To create another account instead, via CLI use `createadmin`. It prompts for a
password on stdin, minimum 12 characters:

```sh
go run ./cmd/alphone createadmin -email you@example.com -name "Your Name"
```

## 4. Run the backend

```sh
make dev
```

`make dev` starts the database container when it is not running yet and
then runs `go run ./cmd/alphone`. Migrations run automatically at
startup, so a fresh database is ready on first boot. The API listens on
`localhost:8080` (change it with `ALPHONE_ADDR`).

The WhatsApp plugin starts with the rest of the app. Without its
`ALPHONE_WHATSAPP_*` variables it runs inert: the screens exist but no
webhook verifies and no message sends. That is fine for everyday
development. To test against a real number, follow
[Meta setup](/whatsapp/meta-setup/) and fill in the `ALPHONE_WHATSAPP_*`
values in your `.env` before starting the backend.

## 5. Run the frontend

In a second terminal, from the repository root:

```sh
pnpm install
pnpm dev
```

`pnpm dev` starts the Vite dev server for the frontend package. It is a
root script for `pnpm --filter @alphone/frontend dev`, which selects
the `@alphone/frontend` package inside the pnpm workspace.

Vite serves the app on [http://localhost:5173](http://localhost:5173) and
proxies every `/api` request to the backend on port 8080 (point it
elsewhere with `ALPHONE_API`). Open it and log in with the admin account
from step 3.

:::note
Always use the `localhost` hostname. The session cookie is `Secure` with
the `__Host-` prefix, and browsers only accept that on HTTPS or on
`localhost`. On a LAN address over plain HTTP the login succeeds but the
cookie is dropped, and you land back on the login screen.
:::

## Serving the built frontend from Go

To exercise the production layout, where the Go binary serves the built
SPA itself:

```sh
make demo
```

That builds the frontend and runs the binary against it, so the whole
app lives on [http://localhost:8080](http://localhost:8080) with no Vite
in front. Use it for manual testing when you want to see exactly what a
release serves. Use `make dev` plus `pnpm dev` when you want hot reload.

## Resetting the database

Manual testing accumulates junk. To get back to the seeded demo data:

```sh
make db-reset
```

That drops every schema AlphOne owns, `core`, `auth`, and one per
plugin, then runs `make seed`, which re-migrates and refills. It
destroys every local record, so never point it at anything you care
about.

## Working on the n8n integration

AlphOne talks to an automation engine over HTTP with an API token. To
run the whole loop on your machine, start the scratch n8n that ships
behind its own compose profile:

```sh
make n8n        # http://localhost:5678
```

The first visit asks you to create an owner account, which is local to
the container and involves no n8n cloud service.

### Install the AlphOne node

```sh
make n8n-node
```

That installs the published
[`n8n-nodes-alphone`](https://www.npmjs.com/package/n8n-nodes-alphone)
from npm and restarts the container, so you exercise the same package a
user installs rather than a local build. It prints the installed version
when it finishes. Two nodes appear afterwards, **AlphOne** and **AlphOne
Trigger**.

To try an unpublished change instead, run `pnpm dev` in the node
repository, which starts its own n8n with the node linked.

### Connect it

Create an **AlphOne API** credential:

| Field | Value |
| ----- | ----- |
| Base URL | `http://host.docker.internal:8080` |
| API Token | a secret from `alphone token create -email you@example.com -name n8n -scope meta:read -scope webhooks:write -scope tasks:write -scope contacts:read`. Without `-scope` it holds every area, and without `-ttl` it lasts ninety days |

Press the test button. It asks AlphOne for its version and should report
success. `localhost` inside the container means the container itself, so
it never reaches AlphOne. See the [automation
guide](/guides/automation/) for the base URL in other layouts.

The backend may listen on loopback only, which is the default. Docker
Desktop forwards loopback to `host.docker.internal`, so it works
unchanged. On Docker Engine for Linux, set `ALPHONE_ADDR=0.0.0.0:8080`
in your `.env` so the container can reach it, and mind that this exposes
the API to your network.

### Prove the loop

Activating a workflow that starts with **AlphOne Trigger** is what
registers the webhook: the node asks AlphOne to create a subscription and
stores its id and signing secret. An inactive trigger has no subscription,
so nothing is delivered.

With a workflow active, create a contact in AlphOne and confirm it
arrives. To check from the AlphOne side:

```sh
docker compose exec postgres psql -U postgres -d postgres -c \
  "SELECT event_name, status, attempts, last_error
     FROM core.webhook_deliveries ORDER BY created_at DESC LIMIT 5"
```

A row reading `delivered` with `attempts = 1` means the whole chain
worked. A row stuck `pending` with `subscriber answered 404` means the
subscription outlived its workflow, which happens when a workflow is
deleted without being deactivated first. List them with the `webhooks`
query and remove the stale one with `deleteWebhook`, otherwise it retries
for a day.

## Running the checks

The repository gates every change on tests and linters:

```sh
make test          # Go tests, needs the database from make db-up
make lint          # golangci-lint plus the docblock linter
cd frontend && pnpm run cover   # frontend tests with 100% coverage thresholds
```

End-to-end tests drive a real browser against a real server. `make e2e`
rebuilds the isolated database, seeds it, and runs Playwright, so every
run starts from the same state. `make e2e-serve` runs the server against
that database on its own, for driving the app by hand.
