// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { catalogueOperation, fieldCatalogueOperation, fieldCatalogueQuery, fieldsQuery } from '../operations'

test('names the catalogue query by the operation it declares', () => {
	const [query] = fieldsQuery.definitions

	expect(query.kind === 'OperationDefinition' ? query.name?.value : undefined).toBe(catalogueOperation)
})

test('names the Fields screen query by the operation it declares', () => {
	const [query] = fieldCatalogueQuery.definitions

	expect(query.kind === 'OperationDefinition' ? query.name?.value : undefined).toBe(fieldCatalogueOperation)
})
