// SPDX-License-Identifier: AGPL-3.0-or-later

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'

import { poeditorAt, syncTranslations } from '@gopherium/gottext/sync'

import { domains, projectVariable, repositoryRoot } from './config.ts'
import { supportedLocales } from './locales.ts'

/** UPLOAD_SPACING_MS is the wait between two uploads. */
const UPLOAD_SPACING_MS = 25_000

/**
 * Waits the given milliseconds.
 * @param ms - How long to wait.
 * @returns A promise settling once the wait is over.
 */
function pause(ms: number): Promise<void> {
	return new Promise((settle) => setTimeout(settle, ms))
}

const token = process.env.POEDITOR_API_TOKEN
if (token === undefined) {
	console.error('set POEDITOR_API_TOKEN to sync translations')
	process.exit(1)
}

const root = repositoryRoot()
const locales = supportedLocales(root)

let uploaded = false

for (const domain of domains()) {
	if (uploaded) {
		await pause(UPLOAD_SPACING_MS)
	}
	uploaded = true
	const variable = projectVariable(domain.name)
	const project = process.env[variable]
	if (project === undefined) {
		console.error(`set ${variable} to sync ${domain.name}`)
		process.exit(1)
	}
	const languages = join(root, domain.languages)
	mkdirSync(languages, { recursive: true })
	const done = await syncTranslations(
		poeditorAt({ token, project, domain: domain.name }),
		locales,
		{
			read: (locale) => {
				const target = join(languages, `${locale}.po`)
				return existsSync(target) ? readFileSync(target, 'utf8') : undefined
			},
			write: (locale, source) => writeFileSync(join(languages, `${locale}.po`), source),
		},
		readFileSync(join(languages, `${domain.name}.pot`), 'utf8'),
	)
	console.log(
		done.moved.length === 0
			? `${domain.name}: no translation moved`
			: `${domain.name}: translations moved: ${done.moved.join(', ')}`,
	)
	for (const held of done.skipped) {
		console.log(`${domain.name}: skipped ${held}`)
	}
	for (const held of done.kept) {
		console.log(`${domain.name}: kept ${held}`)
	}
}
