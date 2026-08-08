// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

export const importsQuery = graphql(`
	query Imports {
		imports {
			id
			filename
			state
			rowCount
			importedCount
			skippedCount
			failedCount
			createdAt
		}
	}
`)

export const importDetailQuery = graphql(`
	query ImportDetail($id: UUID!) {
		importJob(id: $id) {
			id
			filename
			state
			columns
			mapping {
				column
				field
			}
			rows {
				id
				position
				cells
				outcome
				reason
			}
		}
		importFields {
			name
			label
			required
		}
	}
`)
