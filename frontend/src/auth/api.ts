// SPDX-License-Identifier: AGPL-3.0-or-later

import { z } from 'zod'

const userSchema = z.object({
	id: z.string(),
	email: z.string(),
	name: z.string(),
})

export type User = z.infer<typeof userSchema>

/**
 * InvalidCredentialsError is thrown when the backend rejects a login.
 */
export class InvalidCredentialsError extends Error {}

/**
 * UnauthorizedError is thrown when the backend rejects a request because
 * the session is missing, expired, or revoked.
 */
export class UnauthorizedError extends Error {}

/**
 * RateLimitedError is thrown when the backend rejects a login for too many
 * attempts.
 */
export class RateLimitedError extends Error {}

/**
 * Returns the logged-in user, or null when no session is active.
 * @param signal - Aborts the in-flight request.
 * @returns The current user, or null when unauthenticated.
 */
export async function fetchSession(signal?: AbortSignal): Promise<User | null> {
	const response = await fetch('/api/auth/session', { signal })
	if (response.status === 401) {
		return null
	}
	if (!response.ok) {
		throw new Error(`loading session failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Logs in with the given credentials and returns the user.
 * @param email - The account email address.
 * @param password - The account password.
 * @returns The authenticated user.
 */
export async function login(email: string, password: string): Promise<User> {
	const response = await fetch('/api/auth/login', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email, password }),
	})
	if (response.status === 401) {
		throw new InvalidCredentialsError('invalid credentials')
	}
	if (response.status === 429) {
		throw new RateLimitedError('too many login attempts')
	}
	if (!response.ok) {
		throw new Error(`login failed with status ${response.status}`)
	}
	return userSchema.parse(await response.json())
}

/**
 * Ends the current session.
 */
export async function logout(): Promise<void> {
	const response = await fetch('/api/auth/logout', { method: 'POST' })
	if (!response.ok) {
		throw new Error(`logout failed with status ${response.status}`)
	}
}
