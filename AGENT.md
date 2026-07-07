# AlphOne

AlphOne is an open-source multichannel CRM licensed under AGPL-3.0. The backend is a Go service exposing a JSON API; the frontend is a React SPA consuming that API.

## Architecture

- **Plugin-first.** The core contains only the HTTP server, the plugin host, and identity resolution over two entities: `Contact` (the person) and `ContactIdentity` (a per-channel address, unique per channel + identifier). Every feature is a plugin; anything that can be a plugin must be a plugin.
- **Plugins live in one folder each.** A plugin is a directory under `plugins/` holding a `plugin.json` manifest, an ordinary Go package (compiled in), and an optional `frontend/` npm package for its React screens. The Go package exports `Register(sdk.Deps) (*Plugin, error)`; the frontend package exports a `FrontendPlugin` object named `plugin`. `make generate` reads every manifest and regenerates both wiring files; CI fails if they are stale. Each plugin gets a mounted route namespace under `/api/plugins/{name}/` (and `/{name}` in the SPA) and its own Postgres schema with its own migrations. Plugins never import each other and reach the core only through the SDK.

```text
cmd/alphone/          main: config, db pool, plugin registration
cmd/pluginwire/       generator: plugins/*/plugin.json -> wiring files
internal/server       http.Handler, routes, middleware
internal/contact      contact domain package
internal/postgres     data access (pgx + sqlc)
internal/plugin       plugin host, supervisor, registry
plugins/whatsapp      first plugin: Go package + frontend/ React package
sdk/                  public plugin contract (Go) — the only AlphOne import allowed in a plugin
sdk/frontend/         frontend plugin contract, UI facade, and test harness (@alphone/frontend-sdk)
frontend/             React SPA host (Vite); plugins import UI only via @alphone/frontend-sdk
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
