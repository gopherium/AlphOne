// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { poeditorAt } from '@gopherium/gottext/sync'

import { domains, projectVariable, repositoryRoot } from './config.ts'

const token = process.env.POEDITOR_API_TOKEN
if (token === undefined) {
	console.error('set POEDITOR_API_TOKEN to retire terms')
	process.exit(1)
}

const root = repositoryRoot()

const targets = domains().map((domain) => {
	const variable = projectVariable(domain.name)
	const project = process.env[variable]
	if (project === undefined) {
		console.error(`set ${variable} to retire ${domain.name}`)
		process.exit(1)
	}
	return { domain, project }
})

for (const { domain, project } of targets) {
	const template = readFileSync(join(root, domain.languages, `${domain.name}.pot`), 'utf8')
	const deleted = await poeditorAt({ token, project, domain: domain.name }).retireTerms(template)
	console.log(
		deleted === 0
			? `${domain.name}: the platform held nothing to retire`
			: `${domain.name}: terms retired: ${deleted}`,
	)
}
