// SPDX-License-Identifier: AGPL-3.0-or-later

import type { TypedDocumentNode } from '@graphql-typed-document-node/core'
import { print } from 'graphql'

import {
	InvalidCredentialsError,
	InvalidTokenError,
	RateLimitedError,
	UnauthorizedError,
} from '@gopherium/react-auth'
import { ValidationError } from '@gopherium/react-auth/admin'
import type {
	Invitation,
	NewInvite,
	User as BrickAccount,
} from '@gopherium/react-auth/admin'

/**
 * Account is one user account as the admin screens consume it.
 */
export type Account = BrickAccount & { confirmed: boolean }

import {
	acceptInviteMutation,
	inviteMutation,
	loginMutation,
	logoutMutation,
	meQuery,
	requestPasswordResetMutation,
	resendInviteMutation,
	resetPasswordMutation,
	setUserDisabledMutation,
	setUserRoleMutation,
	usersQuery,
} from './operations'

interface GraphError {
	message: string
	extensions?: { code?: string; reason?: string; retryAfter?: number }
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
 * Returns the first error reason of a graph result.
 * @param result - The parsed graph response.
 * @returns The extensions reason, or undefined without errors.
 */
function firstReason<TData>(result: GraphResult<TData>): string | undefined {
	return result.errors?.[0]?.extensions?.reason
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
	confirmed: boolean
	createdAt: string
	role?: string
}): Account {
	return {
		id: user.id,
		email: user.email,
		name: user.name,
		disabled: user.disabled,
		confirmed: user.confirmed,
		created_at: new Date(user.createdAt),
		role: user.role ?? '',
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
 * Maps an invitation answer onto the shape the invite screen consumes.
 * @param report - The delivery report the graph answered.
 * @returns The invitation as the screens consume it.
 */
function toInvitation(report: { delivered: boolean; activationLink?: string | null }): Invitation {
	if (report.delivered) {
		return { delivered: true }
	}
	if (!report.activationLink) {
		throw new Error('the invitation carries no activation link')
	}
	return { delivered: false, activation_link: report.activationLink }
}

/**
 * Refuses an invitation answer by the code the graph carried.
 * @param code - The extensions code of the first error.
 * @param message - The message the error carried.
 */
function refuseInvitation(code: string | undefined, message: string): void {
	if (code === 'UNAUTHENTICATED') {
		throw new UnauthorizedError('session expired')
	}
	if (code === 'RATE_LIMITED') {
		throw new RateLimitedError('too many requests')
	}
	if (code === 'VALIDATION' || code === 'CONFLICT') {
		throw new ValidationError(message)
	}
}

/**
 * Sends an invitation through the invite mutation.
 * @param input - The email, name, and optional role of the invited account.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
async function invite(input: NewInvite): Promise<Invitation> {
	const result = await execute(inviteMutation, {
		email: input.email,
		name: input.name,
		role: input.role ?? null,
	})
	refuseInvitation(firstCode(result), firstMessage(result, 'invalid invitation details'))
	if (!result.data) {
		throw new Error(firstMessage(result, 'inviting failed'))
	}
	return toInvitation(result.data.invite)
}

/**
 * Replaces a pending account's invitation through the resendInvite mutation.
 * @param email - The address the invitation was sent to.
 * @returns The delivery report, carrying the link when nothing mailed it.
 */
export async function resendInvite(email: string): Promise<Invitation> {
	const result = await execute(resendInviteMutation, { email })
	refuseInvitation(firstCode(result), firstMessage(result, 'the invitation could not be sent'))
	if (!result.data) {
		throw new Error(firstMessage(result, 'resending the invitation failed'))
	}
	return toInvitation(result.data.resendInvite)
}

/**
 * Refuses a token operation by the reason the graph carried.
 * @param code - The extensions code of the first error.
 * @param reason - The stable reason of the first error.
 * @param message - The message the error carried.
 */
function refuseToken(code: string | undefined, reason: string | undefined, message: string): void {
	if (code === 'RATE_LIMITED') {
		throw new RateLimitedError('too many attempts')
	}
	if (reason === 'token_invalid') {
		throw new InvalidTokenError('the link is no longer valid')
	}
	if (code === 'VALIDATION') {
		throw new ValidationError(message)
	}
}

/**
 * Accepts an invitation through the acceptInvite mutation.
 * @param token - The activation link's secret.
 * @param password - The password the invited person chooses.
 * @returns The activated and signed-in user.
 */
async function acceptInvite(token: string, password: string) {
	const result = await execute(acceptInviteMutation, { token, password })
	refuseToken(firstCode(result), firstReason(result), firstMessage(result, 'invalid password'))
	if (!result.data) {
		throw new Error(firstMessage(result, 'activation failed'))
	}
	return result.data.acceptInvite.me
}

/**
 * Asks for a reset link through the requestPasswordReset mutation.
 * @param email - The address asking for a reset.
 */
async function requestPasswordReset(email: string): Promise<void> {
	const result = await execute(requestPasswordResetMutation, { email })
	refuseToken(firstCode(result), firstReason(result), firstMessage(result, 'the request failed'))
	if (!result.data) {
		throw new Error(firstMessage(result, 'requesting the reset failed'))
	}
}

/**
 * Replaces the password behind a reset link through the resetPassword mutation.
 * @param token - The reset link's secret.
 * @param password - The password the person chooses.
 */
async function resetPassword(token: string, password: string): Promise<void> {
	const result = await execute(resetPasswordMutation, { token, password })
	refuseToken(firstCode(result), firstReason(result), firstMessage(result, 'invalid password'))
	if (!result.data) {
		throw new Error(firstMessage(result, 'resetting the password failed'))
	}
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
 * Writes the role an account holds through the setUserRole mutation.
 * @param id - The identifier of the account to update.
 * @param role - The role the account is to hold.
 */
export async function setUserRole(id: string, role: string): Promise<void> {
	const result = await execute(setUserRoleMutation, { id, role })
	if (firstCode(result) === 'UNAUTHENTICATED') {
		throw new UnauthorizedError('session expired')
	}
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
	setUserDisabled,
	invite,
	acceptInvite,
	requestPasswordReset,
	resetPassword,
}
