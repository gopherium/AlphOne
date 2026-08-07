// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const contactDetailQuery = graphql(`
	query ContactDetail($id: UUID!, $first: Int, $after: String) {
		contact(id: $id) {
			id
			name
			createdAt
			identities {
				id
				channel
				identifier
				displayName
			}
			tasks(status: "open", first: $first, after: $after) {
				edges {
					node {
						id
						title
						status
						priority
						dueOn
					}
					cursor
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	}
`)

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
