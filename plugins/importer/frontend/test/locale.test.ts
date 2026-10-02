// SPDX-License-Identifier: AGPL-3.0-or-later

import { sprintf } from '@alphone/frontend-sdk'
import { expect, test } from 'vitest'

import { plugin } from '../index'

test('declares its own text domain', () => {
	expect(plugin.locale?.domain).toBe('alphone-importer')
})

test('answers the catalogue for the locale it ships', async () => {
	expect(await plugin.locale?.load('es-ES')).toBeDefined()
})

test('answers no catalogue for a locale shipping none', async () => {
	expect(await plugin.locale?.load('xx-XX')).toBeUndefined()
})

/**
 * Returns what the Spanish catalogue says for each message.
 * @param messages - The messages, a context joined to its message by the catalogue separator.
 * @returns The translations, in the order asked.
 */
async function spanish(...messages: string[]): Promise<unknown[]> {
	const catalog = await plugin.locale?.load('es-ES')
	return messages.map((message) => (catalog?.[message] as string[] | undefined)?.[0])
}

test('reads every import state in Spanish as the state the import is in', async () => {
	expect(
		await spanish('import state\u0004Ready', 'import state\u0004Importing', 'import state\u0004Imported'),
	).toEqual(['Lista', 'En curso', 'Importada'])
})

test('confirms an upload in Spanish with a full stop, beside the action opening it', async () => {
	expect(await spanish('File uploaded.', 'Open', 'Upload')).toEqual(['Archivo subido.', 'Abrir', 'Subir'])
})

test('reads the imports list chrome in Spanish', async () => {
	expect(
		await spanish(
			'Bring contacts in from CSV and Excel files.',
			'Search imports…',
			'No imports found.',
			'Upload a CSV or Excel file to start one.',
		),
	).toEqual([
		'Importa contactos desde archivos CSV y Excel.',
		'Buscar importaciones…',
		'No se ha encontrado ninguna importación.',
		'Sube un archivo CSV o Excel para empezar una.',
	])
})

test('says in Spanish what an import did with no word agreeing with a count', async () => {
	const msgid = 'Import finished: %(imported)s imported, %(skipped)s skipped, %(failed)s failed.'
	const catalog = await plugin.locale?.load('es-ES')
	const [finished] = (catalog?.[msgid] ?? []) as (typeof msgid)[]

	expect(sprintf(finished, { imported: '1.234', skipped: '0', failed: '2' })).toBe(
		'Importación terminada. Importadas: 1.234. Omitidas: 0. Fallidas: 2.',
	)
})
