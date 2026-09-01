import { existsSync, readFileSync } from 'node:fs'
import { join } from 'node:path'

import { poeditorAt, pushTranslations } from '@gopherium/gottext/sync'

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
	console.error('set POEDITOR_API_TOKEN to push translations')
	process.exit(1)
}

const root = repositoryRoot()
const locales = supportedLocales(root)

const targets = domains().map((domain) => {
	const variable = projectVariable(domain.name)
	const project = process.env[variable]?.trim()
	if (project === undefined || project === '') {
		console.error(`set ${variable} to push ${domain.name}`)
		process.exit(1)
	}
	return { domain, project }
})

let uploaded = false

for (const { domain, project } of targets) {
	if (uploaded) {
		await pause(UPLOAD_SPACING_MS)
	}
	uploaded = true
	const languages = join(root, domain.languages)
	const done = await pushTranslations(
		poeditorAt({ token, project, domain: domain.name }),
		locales,
		{
			read: (locale) => {
				const target = join(languages, `${locale}.po`)
				return existsSync(target) ? readFileSync(target, 'utf8') : undefined
			},
			write: () => {},
		},
		readFileSync(join(languages, `${domain.name}.pot`), 'utf8'),
	)
	console.log(
		done.pushed.length === 0
			? `${domain.name}: no catalogue pushed`
			: `${domain.name}: catalogues pushed: ${done.pushed.join(', ')}`,
	)
	for (const held of done.added) {
		console.log(`${domain.name}: added ${held}`)
	}
	for (const held of done.skipped) {
		console.log(`${domain.name}: skipped ${held}`)
	}
}
