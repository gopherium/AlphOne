// SPDX-License-Identifier: AGPL-3.0-or-later

import type { TypedDocumentNode } from '@graphql-typed-document-node/core'
import { print } from 'graphql'

import { InvalidCredentialsError, RateLimitedError, UnauthorizedError } from '@gopherium/react-auth'
import { EmailTakenError, ValidationError } from '@gopherium/react-auth/admin'
import type { NewUser, User as BrickAccount } from '@gopherium/react-auth/admin'

import { type Role, roleOf } from './role'

/**
 * Account is one user account as the admin screens consume it, carrying its tier.
 */
export type Account = BrickAccount & { role: Role }

import {
	createUserMutation,
	loginMutation,
	logoutMutation,
	meQuery,
	setUserDisabledMutation,
	setUserRoleMutation,
	usersQuery,
} from './operations'

interface GraphError {
	message: string
	extensions?: { code?: string; retryAfter?: number }
}

interface GraphResult<TData> {
	data?: TData | null
	errors?: GraphError[]
}

/**
 * Posts one typed operation to the graph endpoint.
 * @param document - The typed operation document.
 * @param variables - The operation variables.
 * @param signal - Aborts the in-flight request.
 * @returns The parsed data and errors envelope.
 */
async function execute<TData, TVariables>(
	document: TypedDocumentNode<TData, TVariables>,
	variables?: TVariables,
	signal?: AbortSignal,
): Promise<GraphResult<TData>> {
	const response = await fetch('/api/graphql', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ query: print(document), variables }),
		signal,
	})
	if (response.status === 429) {
		throw new RateLimitedError('too many requests')
	}
	if (!response.ok) {
		throw new Error(`graphql request failed with status ${response.status}`)
	}
	return (await response.json()) as GraphResult<TData>
}

/**
 * Returns the first error code of a graph result.
 * @param result - The parsed graph response.
 * @returns The extensions code, or undefined without errors.
 */
function firstCode<TData>(result: GraphResult<TData>): string | undefined {
	return result.errors?.[0]?.extensions?.code
}

/**
 * Returns the first error message of a graph result.
 * @param result - The parsed graph response.
 * @param fallback - The message used when no error carries one.
 * @returns The message explaining the failure.
 */
function firstMessage<TData>(result: GraphResult<TData>, fallback: string): string {
	return result.errors?.[0]?.message ?? fallback
}

/**
 * Maps a graph user onto the admin account shape.
 * @param user - The graph user selection.
 * @returns The account as the admin screens consume it.
 */
function toAccount(user: {
	id: string
	email: string
	name: string
	disabled: boolean
	createdAt: string
	role?: string
}): Account {
	return {
		id: user.id,
		email: user.email,
		name: user.name,
		disabled: user.disabled,
		created_at: new Date(user.createdAt),
		role: roleOf(user.role),
	}
}

/**
 * Loads the session through the me query.
 * @param signal - Aborts the in-flight request.
 * @returns The current user, or null when unauthenticated.
 */
async function fetchSession(signal?: AbortSignal) {
	const result = await execute(meQuery, undefined, signal)
	if (firstCode(result) === 'UNAUTHENTICATED') {
		return null
	}
	if (!result.data) {
		throw new Error(firstMessage(result, 'loading session failed'))
	}
	return result.data.me
}

/**
 * Logs in through the login mutation.
 * @param email - The account email address.
 * @param password - The account password.
 * @returns The authenticated user.
 */
async function login(email: string, password: string) {
	const result = await execute(loginMutation, { email, password })
	const code = firstCode(result)
	if (code === 'UNAUTHENTICATED') {
		throw new InvalidCredentialsError('invalid credentials')
	}
	if (code === 'RATE_LIMITED') {
		throw new RateLimitedError('too many login attempts')
	}
	if (!result.data) {
		throw new Error(firstMessage(result, 'login failed'))
	}
	return result.data.login.me
}

/**
 * Ends the session through the logout mutation.
 */
async function logout(): Promise<void> {
	const result = await execute(logoutMutation)
	if (result.errors?.length) {
		throw new Error(firstMessage(result, 'logout failed'))
	}
}

/**
 * Probes the session through the me query.
 * @param signal - Aborts the in-flight request.
 * @returns True when the session is revoked.
 */
async function isSessionRevoked(signal?: AbortSignal): Promise<boolean> {
	const result = await execute(meQuery, undefined, signal)
	return firstCode(result) === 'UNAUTHENTICATED'
}

/**
 * Lists the accounts through the users query.
 * @param signal - Aborts the in-flight request.
 * @returns The accounts as the admin screens consume them.
 */
async function fetchUsers(signal?: AbortSignal): Promise<Account[]> {
	const result = await execute(usersQuery, undefined, signal)
	if (firstCode(result) === 'UNAUTHENTICATED') {
		throw new UnauthorizedError('session expired')
	}
	if (!result.data) {
		throw new Error(firstMessage(result, 'listing users failed'))
	}
	return result.data.users.map(toAccount)
}

/**
 * Creates an account through the createUser mutation.
 * @param input - The email, name, and password of the new account.
 * @returns The created account.
 */
async function createUser(input: NewUser): Promise<Account> {
	const result = await execute(createUserMutation, input)
	const code = firstCode(result)
	if (code === 'UNAUTHENTICATED') {
		throw new UnauthorizedError('session expired')
	}
	if (code === 'CONFLICT') {
		throw new EmailTakenError('email already in use')
	}
	if (code === 'VALIDATION') {
		throw new ValidationError(firstMessage(result, 'invalid user details'))
	}
	if (!result.data) {
		throw new Error(firstMessage(result, 'creating user failed'))
	}
	return toAccount(result.data.createUser)
}

/**
 * Updates an account's disabled flag through the setUserDisabled mutation.
 * @param id - The identifier of the user to update.
 * @param disabled - Whether the account should be disabled.
 */
async function setUserDisabled(id: string, disabled: boolean): Promise<void> {
	const result = await execute(setUserDisabledMutation, { id, disabled })
	const code = firstCode(result)
	if (code === 'UNAUTHENTICATED') {
		throw new UnauthorizedError('session expired')
	}
	if (code === 'VALIDATION') {
		throw new ValidationError(firstMessage(result, 'invalid update'))
	}
	if (result.errors?.length) {
		throw new Error(firstMessage(result, 'updating user failed'))
	}
}

/**
 * Stands one account in another tier through the setUserRole mutation.
 * @param id - The identifier of the user to restand.
 * @param role - The tier the account should stand in.
 */
export async function setUserRole(id: string, role: Role): Promise<void> {
	const result = await execute(setUserRoleMutation, { id, role })
	if (result.errors?.length) {
		throw new Error(firstMessage(result, 'updating the role failed'))
	}
}

/**
 * graphAuthTransport carries every react-auth backend call over the graph.
 */
export const graphAuthTransport = {
	fetchSession,
	login,
	logout,
	isSessionRevoked,
	fetchUsers,
	createUser,
	setUserDisabled,
}
