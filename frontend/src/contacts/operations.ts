// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const createContactMutation = graphql(`
	mutation CreateContact($name: String!) {
		createContact(name: $name) {
			id
			name
		}
	}
`)

export const renameContactMutation = graphql(`
	mutation RenameContact($id: UUID!, $name: String!) {
		renameContact(id: $id, name: $name) {
			id
			name
		}
	}
`)

export const addContactIdentityMutation = graphql(`
	mutation AddContactIdentity($contactId: UUID!, $identity: ContactIdentityInput!) {
		addContactIdentity(contactId: $contactId, identity: $identity) {
			id
			channel
			identifier
			displayName
		}
	}
`)

export const deleteContactIdentityMutation = graphql(`
	mutation DeleteContactIdentity($contactId: UUID!, $identityId: UUID!) {
		deleteContactIdentity(contactId: $contactId, identityId: $identityId)
	}
`)

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
