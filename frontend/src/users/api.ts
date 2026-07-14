// SPDX-License-Identifier: AGPL-3.0-or-later

import { z } from 'zod'

import { UnauthorizedError } from '../auth/api'

export const usersQueryKey = ['users'] as const

const userSchema = z.object({
	id: z.string(),
	email: z.string(),
	name: z.string(),
	disabled: z.boolean(),
	created_at: z.coerce.date(),
})

export type User = z.infer<typeof userSchema>

export interface NewUser {
	email: string
	name: string
	password: string
}

/**
 * EmailTakenError is thrown when a new user's email is already in use.
 */
export class EmailTakenError extends Error {}

/**
 * ValidationError is thrown when the backend rejects the submitted user
 * details; its message is the backend's human-readable explanation.
 */
export class ValidationError extends Error {}

const errorSchema = z.object({ error: z.string() })

/**
 * Extracts the backend's explanation from a failed response body.
 * @param response - The failed HTTP response.
 * @param fallback - The message to use when the body is unreadable.
 * @returns The message explaining the failure.
 */
async function errorMessage(response: Response, fallback: string): Promise<string> {
	try {
		return errorSchema.parse(await response.json()).error
	} catch {
		return fallback
	}
}

/**
 * Returns every user account.
 * @param signal - Aborts the in-flight request.
 * @returns The parsed list of users.
 */
export async function fetchUsers(signal?: AbortSignal): Promise<User[]> {
	const response = await fetch('/api/users', { signal })
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`listing users failed with status ${response.status}`)
	}
	return z.array(userSchema).parse(await response.json())
}

/**
 * Creates a user account and returns it.
 * @param input - The email, name, and password of the new account.
 * @returns The created user.
 */
export async function createUser(input: NewUser): Promise<User> {
	const response = await fetch('/api/users', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(input),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 409) {
		throw new EmailTakenError('email already in use')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, 'invalid user details'))
	}
	if (!response.ok) {
		throw new Error(`creating user failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Sets whether a user may log in.
 * @param id - The identifier of the user to update.
 * @param disabled - Whether the account should be disabled.
 */
export async function setUserDisabled(id: string, disabled: boolean): Promise<void> {
	const response = await fetch(`/api/users/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ disabled }),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`updating user failed with status ${response.status}`)
	}
}
