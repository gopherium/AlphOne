// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from './gql'

export const conversationsQuery = graphql(`
	query WhatsAppConversations {
		whatsAppConversations {
			id
			status
			lastActivityAt
			lastMessagePreview
			contact {
				id
				name
			}
		}
	}
`)
