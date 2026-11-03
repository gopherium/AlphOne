// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { plugin } from '../index'

test('declares its own text domain', () => {
	expect(plugin.locale?.domain).toBe('alphone-whatsapp')
})

test('answers the catalogue for the locale it ships', async () => {
	expect(await plugin.locale?.load('es-ES')).toBeDefined()
})

test('answers no catalogue for a locale shipping none', async () => {
	expect(await plugin.locale?.load('xx-XX')).toBeUndefined()
})

test('speaks Spanish for every word about a number', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect(catalog).toMatchObject({
		'All numbers': ['Todos los números'],
		'Another workspace already holds that number.': ['Otro espacio de trabajo ya tiene ese número.'],
		'Filter by number': ['Filtrar por número'],
		'No conversations on this number.': ['No hay conversaciones en este número.'],
		'Number: %(number)s': ['Número: %(number)s'],
		'That number is no longer connected.': ['Ese número ya no está conectado.'],
		'WhatsApp number': ['Número de WhatsApp'],
	})
})
