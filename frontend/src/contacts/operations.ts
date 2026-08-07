// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const contactsQuery = graphql(`
	query Contacts($q: String, $first: Int, $after: String) {
		contacts(q: $q, first: $first, after: $after) {
			edges {
				node {
					id
					name
					createdAt
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}
`)
