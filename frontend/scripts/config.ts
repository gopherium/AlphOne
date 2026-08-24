// SPDX-License-Identifier: AGPL-3.0-or-later

import { join } from 'node:path'

import type { PotOptions } from '@gopherium/gottext/build'

/** Domain names one text domain beside the sources and catalogues it owns. */
export interface Domain {
	/** name is the text domain every string of this catalogue answers under. */
	name: string
	/** sources are the globs holding the strings, read from the repository root. */
	sources: string[]
	/** languages is the directory the template and its translations sit in. */
	languages: string
	/** catalogs is the directory the compiled catalogues are shipped from. */
	catalogs: string
}

/** IGNORED lists the directories inside every glob that ship no string. */
const IGNORED = ['**/test/**', '**/testdata/**', '**/scripts/**', '**/gql/**']

/** PINNING lists the packages pinning the translation runtime and the brick. */
export const PINNING = ['frontend', 'sdk/frontend']

/**
 * Returns the repository root the source globs resolve against.
 * @returns The absolute path of the repository root.
 */
export function repositoryRoot(): string {
	return join(import.meta.dirname, '..', '..')
}

/**
 * Returns every text domain AlphOne extracts, the core's own first.
 * @returns The domains, in extraction order.
 */
export function domains(): Domain[] {
	return [
		{
			name: 'alphone',
			sources: ['frontend/src/**/*.{ts,tsx}', 'sdk/frontend/**/*.{ts,tsx}'],
			languages: 'languages',
			catalogs: join('frontend', 'src', 'languages'),
		},
		...['fields', 'importer', 'whatsapp'].map((plugin) => ({
			name: `alphone-${plugin}`,
			sources: [`plugins/${plugin}/frontend/**/*.{ts,tsx}`],
			languages: join('plugins', plugin, 'languages'),
			catalogs: join('plugins', plugin, 'frontend', 'languages'),
		})),
	]
}

/**
 * Returns the extraction options one domain's catalogue template builds from.
 * @param domain - The domain to extract.
 * @param sources - The globs to read, the domain's own by default.
 * @returns The options naming the domain, the root and the globs.
 */
export function potConfig(domain: Domain, sources: string[] = domain.sources): PotOptions {
	return {
		domain: domain.name,
		root: repositoryRoot(),
		sources,
		ignored: IGNORED,
	}
}

/**
 * Returns the environment variable naming one domain's platform project.
 * @param domain - The text domain the project holds.
 * @returns The variable name the sync reads the project id from.
 */
export function projectVariable(domain: string): string {
	return `POEDITOR_PROJECT_${domain.toUpperCase().replace(/-/g, '_')}`
}
