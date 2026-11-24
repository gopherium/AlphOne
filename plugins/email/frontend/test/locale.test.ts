// SPDX-License-Identifier: AGPL-3.0-or-later

import { readFileSync } from 'node:fs'
import { join } from 'node:path'

import { expect, test } from 'vitest'

import { plugin } from '../index'

/**
 * Returns the catalogue key of every string the plugin's template names, a context joined before its message.
 * @returns The keys, in the order the template names them.
 */
function templateKeys(): string[] {
	const template = readFileSync(join(import.meta.dirname, '..', '..', 'languages', 'alphone-email.pot'), 'utf8')
	return [...template.matchAll(/(?:msgctxt "(.*)"\n)?msgid "(.+)"/g)].map(([, context, message]) =>
		context === undefined ? message : `${context}\u0004${message}`,
	)
}

test('declares its own text domain', () => {
	expect(plugin.locale?.domain).toBe('alphone-email')
})

test('answers the catalogue for the locale it ships', async () => {
	expect(await plugin.locale?.load('es-ES')).toBeDefined()
})

test('answers no catalogue for a locale shipping none', async () => {
	expect(await plugin.locale?.load('xx-XX')).toBeUndefined()
})

test('names its menu row Correo electrónico in Spanish', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect(catalog?.['admin section\u0004Email']).toEqual(['Correo electrónico'])
})

test('speaks Spanish for every word of the inbox', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect(catalog).toMatchObject({
		'Read the mail of %(address)s in one place.': ['Lee el correo de %(address)s en un solo lugar.'],
		"You're set up. No mail yet.": ['Todo listo. Todavía no hay correo.'],
		'New mail will appear here.': ['El correo nuevo aparecerá aquí.'],
		'No mailbox is connected': ['No hay ningún buzón conectado'],
		'Encrypted': ['Cifrado'],
		'Signed': ['Firmado'],
		'Sent': ['Enviado'],
		'Email': ['Correo electrónico'],
		'Encrypted message': ['Mensaje cifrado'],
		'No text could be read': ['No se ha podido leer el texto'],
		'Loading mail…': ['Cargando el correo…'],
	})
})

test('translates every string its template names', async () => {
	const catalog = (await plugin.locale?.load('es-ES')) ?? {}

	expect(templateKeys().filter((key) => !(catalog[key] as string[] | undefined)?.[0])).toEqual([])
})
