// SPDX-License-Identifier: AGPL-3.0-or-later

import { existsSync, readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'

import { mismatched, orphaned, pot } from '@gopherium/gottext/build'
import { expect, test } from 'vitest'

import { domains, potConfig, projectVariable, repositoryRoot } from '../../scripts/config.ts'
import { supportedLocales } from '../../scripts/locales.ts'

const ROOT = repositoryRoot()

/** committedTemplate reads the template one domain ships. */
function committedTemplate(domain: { name: string; languages: string }): string {
	return readFileSync(join(ROOT, domain.languages, `${domain.name}.pot`), 'utf8')
}

/** translations reads every PO file one domain ships, keyed by locale. */
function translations(domain: { languages: string }): Record<string, string> {
	const held = join(ROOT, domain.languages)
	if (!existsSync(held)) {
		return {}
	}
	const found: Record<string, string> = {}
	for (const file of readdirSync(held)) {
		if (file.endsWith('.po')) {
			found[file.slice(0, -'.po'.length)] = readFileSync(join(held, file), 'utf8')
		}
	}
	return found
}

test('extracts a template for every domain AlphOne ships', () => {
	expect(domains().map((domain) => domain.name)).toEqual([
		'alphone',
		'alphone-fields',
		'alphone-importer',
		'alphone-whatsapp',
	])
})

test('writes the same template bytes on every run', () => {
	for (const domain of domains()) {
		expect(pot(potConfig(domain)).equals(pot(potConfig(domain)))).toBe(true)
	}
})

test('regenerates every committed template byte for byte', () => {
	for (const domain of domains()) {
		expect(pot(potConfig(domain)).toString('utf8')).toBe(committedTemplate(domain))
	}
})

test('names its own domain in every string a domain ships', () => {
	for (const domain of domains()) {
		for (const named of domainsNamedIn(domain.sources)) {
			expect(named).toBe(domain.name)
		}
	}
})

test('carries no translation the template no longer names', () => {
	for (const domain of domains()) {
		for (const [locale, source] of Object.entries(translations(domain))) {
			expect({ locale, orphaned: orphaned(source, committedTemplate(domain)) })
				.toEqual({ locale, orphaned: [] })
		}
	}
})

test('carries no translation naming a placeholder its message does not', () => {
	for (const domain of domains()) {
		for (const [locale, source] of Object.entries(translations(domain))) {
			expect({ locale, mismatched: mismatched(source, committedTemplate(domain)) })
				.toEqual({ locale, mismatched: [] })
		}
	}
})

/** DOMAIN_CALL matches the domain literal a gettext call names last. */
const DOMAIN_CALL = /\b_{1,2}x?\([^)]*?'([a-z-]+)'\s*\)/g

/** domainsNamedIn returns every domain literal the given source globs name. */
function domainsNamedIn(globs: string[]): string[] {
	const named: string[] = []
	for (const file of sourceFiles(globs)) {
		const text = readFileSync(file, 'utf8')
		for (const match of text.matchAll(DOMAIN_CALL)) {
			named.push(match[1])
		}
	}
	return named
}

/** sourceFiles returns every file the given globs reach, tests excluded. */
function sourceFiles(globs: string[]): string[] {
	const roots = globs.map((glob) => join(ROOT, glob.slice(0, glob.indexOf('*')).replace(/\/$/, '')))
	const found: string[] = []
	for (const root of roots) {
		walk(root, found)
	}
	return found.filter((file) => !file.includes('/test/') && !file.includes('/gql/'))
}

/** walk collects every TypeScript source under one directory. */
function walk(directory: string, found: string[]): void {
	if (!existsSync(directory)) {
		return
	}
	for (const entry of readdirSync(directory, { withFileTypes: true })) {
		const path = join(directory, entry.name)
		if (entry.isDirectory()) {
			walk(path, found)
		} else if (path.endsWith('.ts') || path.endsWith('.tsx')) {
			found.push(path)
		}
	}
}

test('reads the languages the server declares, the default first', () => {
	expect(supportedLocales(ROOT)).toEqual(['en-US', 'es-ES'])
})

test('names one platform project per domain', () => {
	expect(domains().map((domain) => projectVariable(domain.name))).toEqual([
		'POEDITOR_PROJECT_ALPHONE',
		'POEDITOR_PROJECT_ALPHONE_FIELDS',
		'POEDITOR_PROJECT_ALPHONE_IMPORTER',
		'POEDITOR_PROJECT_ALPHONE_WHATSAPP',
	])
})
