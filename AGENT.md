# AlphOne

AlphOne is a source-available multichannel CRM. The backend is a Go service exposing a GraphQL API at `POST /api/graphql`; the frontend is a React SPA consuming that API.

## Architecture

- **Plugin-first.** The core contains only the HTTP server, the graph, the plugin host, and identity resolution over two entities: `Contact` (the person) and `ContactIdentity` (a per-channel address, unique per channel + identifier). Every feature is a plugin; anything that can be a plugin must be a plugin.
- **Plugins live in one folder each.** A plugin is a directory under `plugins/` holding a `plugin.json` manifest, an ordinary Go package (compiled in), and an optional `frontend/` npm package for its React screens. The Go package exports `Register(sdk.Deps) (*Plugin, error)`; the frontend package exports a `FrontendPlugin` object named `plugin`. `make generate` runs the whole chain, plugin wiring, sqlc, gqlgen, TypeScript codegen and the schema snapshot; CI fails if any output is stale. A plugin extends the graph with its own schema module, and may still mount HTTP routes under `/api/plugins/{name}/` for what the graph cannot carry. It gets `/{name}` in the SPA and its own Postgres schema with its own migrations. Plugins never import each other and reach the core only through the SDK.

```text
cmd/alphone/          main: config, db pool, plugin registration
cmd/pluginwire/       generator: plugins/*/plugin.json -> wiring files
internal/server       http.Handler, the graph endpoint, middleware
internal/graphres     core graph resolvers
internal/contact      contact domain package
internal/postgres     data access (pgx + sqlc)
plugins/whatsapp      messaging plugin: Go package + frontend/ React package
plugins/importer      contact import plugin: Go package
sdk/                  public plugin contract (Go)
graph/                generated schema and models
sdk/frontend/         frontend plugin contract, UI facade, and test harness (@alphone/frontend-sdk)
frontend/             React SPA host (Vite); plugins import UI only via @alphone/frontend-sdk
```

`sdk/` and `graph/` are the only AlphOne imports allowed in a plugin.

## Stack

- Backend: Go, `net/http` + chi v5, gqlgen (GraphQL), PostgreSQL (pgx/v5, sqlc, goose migrations)
- Frontend: React + TypeScript, Vite, urql + graphcache, TanStack Router + Query, `@wordpress/ui` + `@wordpress/theme` (WordPress Design System on Base UI), Storybook
- Testing: stdlib table-driven tests, httptest, pgtestdb (backend); Vitest, Testing Library, MSW (frontend)

## Contributing

1. Keep changes small and focused: one behavior per change.
2. Every change ships with tests, written before the implementation.
3. Every function carries a doc comment: Go in canonical form, TypeScript following tsdoc standard. Lines wrap at 120 columns.
4. Run `make test`, `make lint`, and `make vuln` before submitting. CI enforces all three, plus the race detector and SDK compatibility checks. `make vuln` fails only on advisories your code actually calls.

## License

Split: backend under the Elastic License 2.0 ([LICENSE](LICENSE)); frontend
packages under AGPL-3.0-or-later ([frontend/LICENSE](frontend/LICENSE)).
Every source file must carry an `SPDX-License-Identifier` header naming its
license.
