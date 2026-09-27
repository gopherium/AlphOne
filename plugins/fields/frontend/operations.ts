// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

/** catalogueOperation names the query reading the field catalogue. */
export const catalogueOperation = 'Fields'

export const fieldsQuery = graphql(`
	query Fields {
		fields {
			id
			name
			label
			kind
			subFields {
				name
				label
				kind
			}
		}
	}
`)

export const defineFieldMutation = graphql(`
	mutation DefineField(
		$name: String!
		$label: String!
		$kind: FieldKind!
		$subFields: [FieldSubFieldInput!]
	) {
		defineField(name: $name, label: $label, kind: $kind, subFields: $subFields) {
			id
			name
			label
			kind
			subFields {
				name
				label
				kind
			}
		}
	}
`)

export const archiveFieldMutation = graphql(`
	mutation ArchiveField($id: UUID!) {
		archiveField(id: $id)
	}
`)

export const writeContactFieldsMutation = graphql(`
	mutation WriteContactFields($contactId: UUID!, $values: JSON!) {
		writeContactFields(contactId: $contactId, values: $values)
	}
`)

export const addContactFieldEntryMutation = graphql(`
	mutation AddContactFieldEntry($contactId: UUID!, $field: String!, $entry: JSON!) {
		addContactFieldEntry(contactId: $contactId, field: $field, entry: $entry)
	}
`)

export const updateContactFieldEntryMutation = graphql(`
	mutation UpdateContactFieldEntry($contactId: UUID!, $field: String!, $entryId: UUID!, $entry: JSON!) {
		updateContactFieldEntry(contactId: $contactId, field: $field, entryId: $entryId, entry: $entry)
	}
`)

export const deleteContactFieldEntryMutation = graphql(`
	mutation DeleteContactFieldEntry($contactId: UUID!, $field: String!, $entryId: UUID!) {
		deleteContactFieldEntry(contactId: $contactId, field: $field, entryId: $entryId)
	}
`)
