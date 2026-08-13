// SPDX-License-Identifier: AGPL-3.0-or-later

const selectable = /^[a-z][a-zA-Z0-9]*$/

/**
 * Returns the query reading the given fields of one contact.
 * @param names - The field names the catalogue holds.
 * @returns The GraphQL document selecting those fields.
 */
export function contactValuesDocument(names: string[]) {
	const selections = names.filter((name) => selectable.test(name)).join('\n\t\t\t')
	return `query ContactFieldValues($id: UUID!) {
		contact(id: $id) {
			id
			${selections}
		}
	}`
}
