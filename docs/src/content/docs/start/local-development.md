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

Create the admin account. The command prompts for a password on stdin,
minimum 12 characters:

```sh
go run ./cmd/alphone createadmin -email you@example.com -name "Your Name"
```

## 4. Run the backend

```sh
go run ./cmd/alphone
```

Migrations run automatically at startup, so a fresh database is ready on
first boot. The API listens on `localhost:8080` (change it with
`ALPHONE_ADDR`).

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
pnpm --filter @alphone/frontend dev
```

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
SPA itself, build the frontend and point the backend at the output:

```sh
pnpm --filter @alphone/frontend build
ALPHONE_WEB_DIR=frontend/dist go run ./cmd/alphone
```

Then the whole app lives on [http://localhost:8080](http://localhost:8080)
with no Vite in front.

## Running the checks

The repository gates every change on tests and linters:

```sh
make test          # Go tests, needs the database from make db-up
make lint          # golangci-lint plus the docblock linter
cd frontend && pnpm run cover   # frontend tests with 100% coverage thresholds
```

End-to-end tests drive a real browser against a real server. See the
Makefile `e2e-*` targets: `make e2e-reset` seeds an isolated database,
`make e2e-serve` runs the server against it, and `make e2e` runs
Playwright.
