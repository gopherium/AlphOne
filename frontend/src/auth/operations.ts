// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const meQuery = graphql(`
	query Me {
		me {
			id
			email
			name
			role
			capabilities
			grantable
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
				capabilities
				grantable
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
			confirmed
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

export const setUserDisabledMutation = graphql(`
	mutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {
		setUserDisabled(id: $id, disabled: $disabled)
	}
`)

export const inviteMutation = graphql(`
	mutation Invite($email: String!, $name: String!, $role: String) {
		invite(email: $email, name: $name, role: $role) {
			delivered
			activationLink
		}
	}
`)

export const resendInviteMutation = graphql(`
	mutation ResendInvite($email: String!) {
		resendInvite(email: $email) {
			delivered
			activationLink
		}
	}
`)

export const acceptInviteMutation = graphql(`
	mutation AcceptInvite($token: String!, $password: String!) {
		acceptInvite(token: $token, password: $password) {
			me {
				id
				email
				name
				role
				capabilities
				grantable
			}
		}
	}
`)

export const requestPasswordResetMutation = graphql(`
	mutation RequestPasswordReset($email: String!) {
		requestPasswordReset(email: $email)
	}
`)

export const resetPasswordMutation = graphql(`
	mutation ResetPassword($token: String!, $password: String!) {
		resetPassword(token: $token, password: $password)
	}
`)
