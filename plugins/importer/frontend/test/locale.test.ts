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

test('names the contact fields in Spanish as the contact screens do', async () => {
	expect(
		await spanish('contact field\u0004Name', 'contact field\u0004Email', 'contact field\u0004Phone'),
	).toEqual(['Nombre', 'Correo electrónico', 'Teléfono'])
})

test('says in Spanish why each staged row settled as it did', async () => {
	expect(
		await spanish(
			'The row does not match the header. Cells in the row: %(cells)s. Columns in the header: %(columns)s.',
			'Line %(line)s of the file has a quote mark out of place, so this row was left empty.',
			'AlphOne could not read this row, so it was left empty.',
			'The row has no name or no address.',
			'The row holds a name or an address AlphOne cannot use.',
			'Another contact already holds an address in this row.',
			'%(ownerName)s already holds an address in this row.',
			'The value for %(field)s does not match the kind the field declares: %(kind)s.',
			'Fields that no longer exist: %(fields)s.',
			'A field does not accept the value this row holds.',
		),
	).toEqual([
		'La fila no encaja con la cabecera. Celdas en la fila: %(cells)s. Columnas en la cabecera: %(columns)s.',
		'La línea %(line)s del archivo tiene unas comillas mal colocadas, así que esta fila ha quedado vacía.',
		'AlphOne no ha podido leer esta fila, así que ha quedado vacía.',
		'La fila no tiene nombre o no tiene ninguna dirección.',
		'La fila tiene un nombre o una dirección que AlphOne no puede usar.',
		'Ya hay otro contacto que tiene una dirección de esta fila.',
		'%(ownerName)s ya tiene una dirección de esta fila.',
		'El valor de %(field)s no encaja con el tipo que declara el campo: %(kind)s.',
		'Campos que ya no existen: %(fields)s.',
		'Un campo no admite el valor que tiene esta fila.',
	])
})

test('names every field kind in Spanish as the fields screen does', async () => {
	expect(
		await spanish(
			'field kind\u0004Text',
			'field kind\u0004Long text',
			'field kind\u0004Number',
			'field kind\u0004Yes or no',
			'field kind\u0004Date',
			'field kind\u0004Choice',
			'field kind\u0004Repeater',
		),
	).toEqual(['Texto', 'Texto largo', 'Número', 'Sí o no', 'Fecha', 'Elección', 'Repetidor'])
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
