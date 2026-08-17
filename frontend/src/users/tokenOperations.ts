// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const apiTokensQuery = graphql(`
	query ApiTokens {
		apiTokens {
			id
			name
			scopes
			createdAt
			lastUsedAt
			expiresAt
		}
	}
`)

export const apiTokenCreateMutation = graphql(`
	mutation ApiTokenCreate($name: String!, $scopes: [String!]!, $ttlDays: Int) {
		apiTokenCreate(name: $name, scopes: $scopes, ttlDays: $ttlDays) {
			secret
			token {
				id
				name
				scopes
				createdAt
				lastUsedAt
				expiresAt
			}
		}
	}
`)

export const apiTokenRevokeMutation = graphql(`
	mutation ApiTokenRevoke($id: UUID!) {
		apiTokenRevoke(id: $id)
	}
`)
