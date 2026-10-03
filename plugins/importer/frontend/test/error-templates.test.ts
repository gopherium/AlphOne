// SPDX-License-Identifier: AGPL-3.0-or-later

import { sprintf } from '@alphone/frontend-sdk'
import { expect, test } from 'vitest'

import { errorTemplates } from '../errorTemplates'
import { plugin } from '../index'

test('writes the upload limit as the text the format locale made of it', () => {
	const template = errorTemplates().file_too_large as '%(maxBytes)s'

	expect(sprintf(template, { maxBytes: '5.242.880' })).toBe('The file runs past 5.242.880 bytes.')
})

test('takes the upload limit as text in the Spanish message too', async () => {
	const catalog = await plugin.locale?.load('es-ES')

	expect(catalog?.['The file runs past %(maxBytes)s bytes.']).toEqual(['El archivo pasa de %(maxBytes)s bytes.'])
})
