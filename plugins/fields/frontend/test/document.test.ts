// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { contactValuesDocument } from '../document'

test('a document selects every defined field by its own name', () => {
	const document = contactValuesDocument(['birthDate', 'loyaltyPoints'])

	expect(document).toContain('birthDate')
	expect(document).toContain('loyaltyPoints')
	expect(document).toContain('$id: UUID!')
})

test('no defined fields select nothing beyond the contact id', () => {
	const document = contactValuesDocument([])

	expect(document).toContain('id')
	expect(document).not.toContain('birthDate')
})

test('a name the catalogue could never hold is left out', () => {
	const document = contactValuesDocument(['birthDate', 'drop me', '__typename', ''])

	expect(document).toContain('birthDate')
	expect(document).not.toContain('drop me')
	expect(document).not.toContain('__typename')
})
