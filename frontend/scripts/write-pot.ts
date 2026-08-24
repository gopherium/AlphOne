// SPDX-License-Identifier: AGPL-3.0-or-later

import { mkdirSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { pot } from '@gopherium/gottext/build'

import { domains, potConfig, repositoryRoot } from './config.ts'

const root = repositoryRoot()

for (const domain of domains()) {
	const languages = join(root, domain.languages)
	mkdirSync(languages, { recursive: true })
	writeFileSync(join(languages, `${domain.name}.pot`), pot(potConfig(domain)))
}
