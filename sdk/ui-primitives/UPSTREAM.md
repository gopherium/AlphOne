# Upstream ledger

Every primitive here exists because `@wordpress/ui` does not ship it yet. Each
one is written to be replaced: same name, same anatomy, same prop conventions as
the upstream component we expect, built on the same `@base-ui/react` version
`@wordpress/ui` builds on. When upstream ships the real one, the swap is a single
line in the facade at `sdk/frontend/index.ts`.

Kept domain free on purpose. Nothing here knows about contacts, tasks, or
WhatsApp, so any of it can be offered upstream as written.

Review this file whenever the `@wordpress/ui` pin moves.

## Primitives

| Primitive | Why it is here | Upstream status | Replace when |
| --- | --- | --- | --- |
| `Checkbox` | Not in `@wordpress/ui` 0.17.0. The task list needs the canonical to-do affordance, and an icon button reading as a tick is a poor substitute. | Not shipped. Base UI provides `Checkbox.Root` and `Checkbox.Indicator`, so upstream would be layering, not building from scratch. | `@wordpress/ui` exports a `Checkbox`. Compare its anatomy against ours first, then flip the facade export and delete this file's entry. |

## Findings worth reporting upstream

| Finding | Where | Notes |
| --- | --- | --- |
| `Notice.Root` announces through `@wordpress/a11y`, so its text appears twice in the DOM. Consumer test suites cannot use plain text queries without ignoring the live region. | `@wordpress/ui` 0.17.0 Notice | Discussed in WordPress/gutenberg#80706, where a reviewer proposes the primitive should carry no live region at all. We work around it with `configure({ defaultIgnore })` in `frontend/src/test/setup.ts`. Real external-consumer evidence for that thread. |
| `@wordpress/element` named-imports React DOM APIs removed in React 19, so it crashes on load under React 19. | `@wordpress/element` 8.2.0 | Patched locally through pnpm `patchedDependencies`. The patch file is the pull request draft. |

## Conventions to keep

- Build on `@base-ui/react`, matching the version `@wordpress/ui` resolves, so a
  single deduped instance stays in the tree.
- Style with `--wpds-*` tokens only. Never define or override a `--wpds-*`
  custom property. Anything of our own goes under `--a1-*`.
- Put styles in an `alphone` cascade layer so they order predictably against the
  `wp-ui` layer.
- Mirror upstream prop conventions: `defaultX` with `x` and `onXChange`,
  compound parts named `Root` and so on, and pass through the underlying event
  details rather than flattening them.
