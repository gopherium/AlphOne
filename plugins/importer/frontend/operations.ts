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
