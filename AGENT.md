# AlphOne

AlphOne is an open-source multichannel CRM licensed under AGPL-3.0. The backend is a Go service exposing a JSON API; the frontend is a React SPA consuming that API.

## Architecture

- **Plugin-first.** The core contains only the HTTP server, the plugin host, and identity resolution over two entities: `Contact` (the person) and `ContactIdentity` (a per-channel address, unique per channel + identifier). Every feature is a plugin; anything that can be a plugin must be a plugin.
- **Plugins are ordinary Go packages**, compiled in and explicitly registered in `cmd/alphone`. Each plugin gets a mounted route namespace under `/api/plugins/{name}/` and its own Postgres schema with its own migrations. Plugins never import each other and reach the core only through the plugin API.

```text
cmd/alphone/          main: config, db pool, plugin registration
internal/server       http.Handler, routes, middleware
internal/contact      contact domain package
internal/postgres     data access (pgx + sqlc)
internal/plugin       plugin host, supervisor, registry
plugins/whatsapp      first plugin
sdk/                  public plugin contract — the only AlphOne import allowed in a plugin
frontend/             React SPA (Vite); app code imports UI components only via src/ui
```

## Stack

- Backend: Go, `net/http` + chi v5, PostgreSQL (pgx/v5, sqlc, goose migrations)
- Frontend: React + TypeScript, Vite, TanStack Router + Query, `@wordpress/ui` + `@wordpress/theme` (WordPress Design System on Base UI), Storybook
- Testing: stdlib table-driven tests, httptest, pgtestdb (backend); Vitest, Testing Library, MSW (frontend)

## Contributing

1. Keep changes small and focused: one behavior per change.
2. Every change ships with tests, written before the implementation.
3. Exported identifiers carry doc comments.
4. Run `make test` and `make lint` before submitting; CI enforces both, plus the race detector and SDK compatibility checks.

## License

AGPL-3.0. See [LICENSE](LICENSE) for the full text.
