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

export const importUploadMutation = graphql(`
	mutation ImportUpload($file: Upload!) {
		importUpload(file: $file) {
			id
			filename
		}
	}
`)

export const importSetMappingMutation = graphql(`
	mutation ImportSetMapping($id: UUID!, $assignments: [ImportAssignmentInput!]!) {
		importSetMapping(id: $id, assignments: $assignments) {
			id
			state
			mapping {
				column
				field
			}
		}
	}
`)

export const importCommitMutation = graphql(`
	mutation ImportCommit($id: UUID!) {
		importCommit(id: $id) {
			id
			imported
			skipped
			failed
		}
	}
`)
