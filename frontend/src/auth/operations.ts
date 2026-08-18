// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const meQuery = graphql(`
	query Me {
		me {
			id
			email
			name
			role
		}
	}
`)

export const loginMutation = graphql(`
	mutation Login($email: String!, $password: String!) {
		login(email: $email, password: $password) {
			me {
				id
				email
				name
				role
			}
		}
	}
`)

export const logoutMutation = graphql(`
	mutation Logout {
		logout
	}
`)

export const usersQuery = graphql(`
	query Users {
		users {
			id
			email
			name
			disabled
			createdAt
			role
		}
	}
`)

export const setUserRoleMutation = graphql(`
	mutation SetUserRole($id: UUID!, $role: String!) {
		setUserRole(id: $id, role: $role)
	}
`)

export const createUserMutation = graphql(`
	mutation CreateUser($email: String!, $name: String!, $password: String!) {
		createUser(email: $email, name: $name, password: $password) {
			id
			email
			name
			disabled
			createdAt
		}
	}
`)

export const setUserDisabledMutation = graphql(`
	mutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {
		setUserDisabled(id: $id, disabled: $disabled)
	}
`)
