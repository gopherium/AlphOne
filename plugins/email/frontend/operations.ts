// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

export const mailboxQuery = graphql(`
	query EmailMailbox {
		emailMailbox {
			id
			address
			aliases
		}
	}
`)

export const inboxQuery = graphql(`
	query EmailInbox($limit: Int, $offset: Int) {
		emailThreads(limit: $limit, offset: $offset) {
			total
			limit
			items {
				id
				subject
				lastMessageAt
				messageCount
				hasAttachments
				latest {
					id
					from {
						name
						email
					}
					snippet
					direction
					encrypted
					signed
					bodyState
				}
			}
		}
	}
`)
