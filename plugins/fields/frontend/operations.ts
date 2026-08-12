// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

export const fieldsQuery = graphql(`
	query Fields {
		fields {
			id
			name
			label
			kind
		}
	}
`)

export const defineFieldMutation = graphql(`
	mutation DefineField($name: String!, $label: String!, $kind: FieldKind!) {
		defineField(name: $name, label: $label, kind: $kind) {
			id
			name
			label
			kind
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
