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

None. The package stays as the scaffold for the next one.

## Replaced by upstream

| Primitive | Lived here | Replaced by | Notes |
| --- | --- | --- | --- |
| `Checkbox` | `@wordpress/ui` 0.17.0 | `@wordpress/ui` 0.18.0 | WordPress/gutenberg#80039 landed the same anatomy we had built: `Checkbox.Root` with an indicator carrying the `check` icon. Upstream takes no `label` prop, so callers pass `aria-label`. The facade swap was one line. |

## Findings worth reporting upstream

| Finding | Where | Notes |
| --- | --- | --- |
| `Notice.Root` announces through `@wordpress/a11y`, so its text appears twice in the DOM. Consumer test suites cannot use plain text queries without ignoring the live region. | `@wordpress/ui` 0.17.0 Notice | Discussed in WordPress/gutenberg#80706, where a reviewer proposes the primitive should carry no live region at all. We work around it with `configure({ defaultIgnore })` in `frontend/src/test/setup.ts`. Real external-consumer evidence for that thread. |
| `@wordpress/element` named-imports React DOM APIs removed in React 19, so it crashes on load under React 19. | `@wordpress/element` 8.4.0 | Patched locally through pnpm `patchedDependencies`. The patch file is the pull request draft. Present since 8.2.0 at least, and the package still requires `react: ^18.3.1`. |

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
