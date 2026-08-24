// SPDX-License-Identifier: AGPL-3.0-or-later

import { existsSync, mkdirSync, readFileSync, readdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { compileCatalog, serializeCatalog } from '@gopherium/gottext/build'

import { domains, repositoryRoot } from './config.ts'

const root = repositoryRoot()

for (const domain of domains()) {
	const sources = join(root, domain.languages)
	const target = join(root, domain.catalogs)
	if (!existsSync(sources)) {
		continue
	}
	mkdirSync(target, { recursive: true })
	for (const file of readdirSync(sources)) {
		if (!file.endsWith('.po')) {
			continue
		}
		const locale = file.slice(0, -'.po'.length)
		const compiled = compileCatalog(readFileSync(join(sources, file), 'utf8'))
		writeFileSync(join(target, `${locale}.json`), serializeCatalog(compiled))
	}
}
