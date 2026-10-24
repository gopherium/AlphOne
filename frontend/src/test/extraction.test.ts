// SPDX-License-Identifier: AGPL-3.0-or-later

import { existsSync, globSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { basename, join } from 'node:path'

import { mismatched, orphaned, pot } from '@gopherium/gottext/build'
import { sprintf } from '@wordpress/i18n'
import { expect, onTestFinished, test } from 'vitest'

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

/** FIXTURE_MANIFESTS are the plugin manifests a temporary repository holds, keyed by folder. */
const FIXTURE_MANIFESTS = {
	'plugins/demo': { id: 'demo', name: 'Demo', frontend: '@alphone/plugin-demo' },
	'plugins/quiet': { id: 'quiet', name: 'Quiet' },
	'enterprise/extra': { id: 'extra', name: 'Extra', frontend: '@alphone/plugin-extra' },
}

/** manifestTree writes a temporary repository holding the given plugin manifests. */
function manifestTree(manifests: Record<string, object>): string {
	const root = mkdtempSync(join(tmpdir(), 'alphone-domains-'))
	onTestFinished(() => rmSync(root, { recursive: true, force: true }))
	for (const [folder, manifest] of Object.entries(manifests)) {
		mkdirSync(join(root, folder), { recursive: true })
		writeFileSync(join(root, folder, 'plugin.json'), JSON.stringify(manifest))
	}
	return root
}

test('finds one domain per plugin manifest that names a frontend', () => {
	const found = domains(manifestTree(FIXTURE_MANIFESTS))

	expect(found.map((domain) => domain.name)).toEqual(['alphone', 'alphone-demo'])
})

test('extracts a template for every domain AlphOne ships', () => {
	const shipped = globSync('plugins/*/languages/*.pot', { cwd: ROOT }).map((path) => basename(path, '.pot'))

	expect(domains().map((domain) => domain.name)).toEqual(['alphone', ...shipped.sort()])
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
	const found = domains(manifestTree(FIXTURE_MANIFESTS))

	expect(found.map((domain) => projectVariable(domain.name))).toEqual([
		'POEDITOR_PROJECT_ALPHONE',
		'POEDITOR_PROJECT_ALPHONE_DEMO',
	])
})

test('renders every translation the platform returned', () => {
	for (const domain of domains()) {
		for (const source of Object.values(translations(domain))) {
			for (const rendered of renderable(source)) {
				expect(() => sprintf(rendered, sampleFor(rendered) as never)).not.toThrow()
			}
		}
	}
})

/** RENDER_PAIRS matches one translated entry beside the message it stands for. */
const RENDER_PAIRS = /msgid "((?:[^"\\]|\\.)*)"\nmsgstr "((?:[^"\\]|\\.)*)"/g

/** renderable returns every translation a catalogue actually carries. */
function renderable(source: string): string[] {
	const found: string[] = []
	for (const match of source.matchAll(RENDER_PAIRS)) {
		if (match[1] !== '' && match[2] !== '') {
			found.push(match[2])
		}
	}
	return found
}

/** sampleFor returns a stand-in value for every placeholder a message names. */
function sampleFor(message: string): Record<string, unknown> {
	const values: Record<string, unknown> = {}
	for (const match of message.matchAll(/%\(([$_a-zA-Z][$_a-zA-Z0-9]*)\)[ +0#-]*\d*(?:\.\d+)?([a-z])/g)) {
		values[match[1]] = match[2] === 'd' || match[2] === 'f' ? 1 : 'x'
	}
	return values
}
