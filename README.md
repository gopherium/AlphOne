# AlphOne

[![codecov](https://codecov.io/github/gopherium/AlphOne/graph/badge.svg?token=FGCAJXGLE7)](https://codecov.io/github/gopherium/AlphOne)

A plugin-first CRM. Go backend exposing a JSON API, React SPA frontend.

## Documentation

Setup, self-hosting, and reference guides live at
[docs.alph.one](https://docs.alph.one/).

## Translating

The interface speaks whatever language its catalogues carry. Translation
happens on [POEditor](https://poeditor.com/join/project/1skIO0ryto), no Git
needed, and a weekly job carries the finished work back in one batch.
[Translate AlphOne](https://docs.alph.one/contributing/translate-alphone/)
walks it, and names the separate project each plugin keeps.

## License

Copyright (C) 2026 Manuel 'SirLouen' Camargo

AlphOne is source-available software under a split license:

- **Backend** (all Go code and SQL migrations): the
  [Elastic License 2.0](LICENSE). You may use, copy, modify, and
  redistribute it, but you may not provide AlphOne to third parties as a
  hosted or managed service.
- **Frontend, tests and docs** (`frontend/`, `sdk/frontend/`,
  `sdk/ui-primitives/`, `plugins/*/frontend/`, `test/e2e/`, `docs/`,
  `codegen.ts`, `eslint.config.js`): the
  [GNU Affero General Public License v3.0 or later](frontend/LICENSE).

Each source file must carry an `SPDX-License-Identifier` header naming its
license.
