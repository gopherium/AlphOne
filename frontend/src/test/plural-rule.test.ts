// SPDX-License-Identifier: AGPL-3.0-or-later

/// <reference types="node" />
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { _n, formatNumber, rememberFormatLocale, sprintf } from '@alphone/frontend-sdk'
import { compileCatalog, serializeCatalog } from '@gopherium/gottext/build'
import { withPluralRuleOf } from '@gopherium/gottext/sync'
import { resetLocaleData, setLocaleData } from '@wordpress/i18n'
import type { LocaleData } from '@wordpress/i18n'
import { expect, onTestFinished, test } from 'vitest'

import { domains, repositoryRoot } from '../../scripts/config.ts'

/** spanishRule is the CLDR plural rule for Spanish, the one the translation platform exports. */
const spanishRule = 'nplurals=3; plural=((n == 1) ? 0 : ((n != 0 && n % 1000000 == 0) ? 1 : 2));'

/** countPrinted matches a message whose text prints the count. */
const countPrinted = /%(\(\w+\))?[sd]/

/** pluralMessages are the plural messages the core screens use. */
const pluralMessages = [
	'%s user could not be disabled.',
	'%s user could not be enabled.',
	'%s user disabled.',
	'%s user enabled.',
	'%s token could not be revoked.',
	'%s token revoked.',
	'Revoke %s token? This cannot be undone.',
]

/** threeFormExport is a platform export carrying the Spanish rule and its three forms filled. */
const threeFormExport = [
	'msgid ""',
	'msgstr ""',
	'"Language: es\\n"',
	`"Plural-Forms: ${spanishRule}\\n"`,
	'',
	'msgid "%s contact"',
	'msgid_plural "%s contacts"',
	'msgstr[0] "%s contacto"',
	'msgstr[1] "%s de contactos"',
	'msgstr[2] "%s contactos"',
	'',
].join('\n')

/** Compiled is a catalogue as the runtime loads it, its header under the empty key. */
type Compiled = Record<string, unknown>

/** SpanishCatalogue is one text domain's committed Spanish catalogue beside the compiled one it ships. */
interface SpanishCatalogue {
	domain: string
	po: string
	shipped: string
}

/**
 * Returns the committed and the shipped Spanish catalogue of every text domain.
 * @returns The catalogues, one per domain.
 */
function spanishCatalogues(): SpanishCatalogue[] {
	return domains().map((domain) => ({
		domain: domain.name,
		po: readFileSync(join(repositoryRoot(), domain.languages, 'es-ES.po'), 'utf8'),
		shipped: readFileSync(join(repositoryRoot(), domain.catalogs, 'es-ES.json'), 'utf8'),
	}))
}

/**
 * Returns the catalogue a PO text compiles to.
 * @param po - The catalogue as PO text.
 * @returns The compiled catalogue.
 */
function compiled(po: string): Compiled {
	return JSON.parse(serializeCatalog(compileCatalog(po))) as Compiled
}

/**
 * Returns the plural rule a compiled catalogue declares.
 * @param catalogue - The compiled catalogue.
 * @returns The rule.
 */
function ruleOf(catalogue: Compiled): unknown {
	return (catalogue[''] as Record<string, unknown>)['plural-forms']
}

/**
 * Returns every plural message of the Spanish catalogues with its forms.
 * @param domain - The one text domain to read, every domain when left out.
 * @returns The messages, each with its forms.
 */
function plurals(domain?: string): [string, string[]][] {
	return spanishCatalogues()
		.filter((catalogue) => domain === undefined || catalogue.domain === domain)
		.flatMap(({ po }) =>
			Object.entries(compiled(po)).filter(
				(entry): entry is [string, string[]] => Array.isArray(entry[1]) && entry[1].length > 1,
			),
		)
}

test('every Spanish catalogue declares the CLDR plural rule', () => {
	for (const { domain, po } of spanishCatalogues()) {
		expect(ruleOf(compiled(po)), domain).toBe(spanishRule)
	}
})

test('every Spanish catalogue ships as its PO text compiles', () => {
	for (const { domain, po, shipped } of spanishCatalogues()) {
		expect(shipped, domain).toBe(serializeCatalog(compileCatalog(po)))
	}
})

test('the core Spanish catalogue carries exactly the core plural messages', () => {
	expect(new Set(plurals('alphone').map(([message]) => message))).toEqual(new Set(pluralMessages))
})

test('every Spanish plural message fills its three forms', () => {
	for (const [message, forms] of plurals()) {
		expect(forms, message).toHaveLength(3)
		expect(forms.filter((form) => form === ''), message).toEqual([])
	}
})

test('a whole number of millions has its own words wherever the count is printed', () => {
	for (const [message, forms] of plurals()) {
		if (countPrinted.test(message)) {
			expect(forms[1], message).not.toBe(forms[2])
		}
	}
})

test('a three form export keeps every form against every catalogue', () => {
	for (const { domain, po } of spanishCatalogues()) {
		const kept = compiled(withPluralRuleOf(po, threeFormExport))

		expect({ domain, rule: ruleOf(kept), forms: kept['%s contact'] }).toEqual({
			domain,
			rule: spanishRule,
			forms: ['%s contacto', '%s de contactos', '%s contactos'],
		})
	}
})

test.each([
	[1, '1 token revocado.'],
	[2, '2 tokens revocados.'],
	[1000000, '1.000.000 de tokens revocados.'],
	[1500000, '1.500.000 tokens revocados.'],
	[2000000, '2.000.000 de tokens revocados.'],
])('revoking %s tokens reads in Spanish as %s', (count, words) => {
	const core = spanishCatalogues().find(({ domain }) => domain === 'alphone')
	setLocaleData(compiled(core?.po ?? '') as LocaleData, 'alphone')
	onTestFinished(() => resetLocaleData(undefined, 'alphone'))
	rememberFormatLocale('es-ES')

	expect(sprintf(_n('%s token revoked.', '%s tokens revoked.', count, 'alphone'), formatNumber(count))).toBe(words)
})
