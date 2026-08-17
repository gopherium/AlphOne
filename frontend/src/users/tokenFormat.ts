// SPDX-License-Identifier: AGPL-3.0-or-later

import { formatCreated } from '../contacts/format'

/** grantableAreas lists the areas a token may be granted, in menu order. */
export const grantableAreas = ['contacts', 'tasks', 'users', 'webhooks'] as const

/** lifetimeChoices lists the lifetimes the mint form offers, in menu order. */
export const lifetimeChoices = [
	{ value: '30', label: '30 days' },
	{ value: '90', label: '90 days' },
	{ value: '365', label: 'A year' },
	{ value: 'never', label: 'Never' },
]

/** defaultLifetime is the lifetime the mint form starts on. */
export const defaultLifetime = '90'

/**
 * Formats the moment a token was last presented.
 * @param at - The moment, or null when it never acted.
 * @returns The date, or a plain statement that it never acted.
 */
export function formatLastUsed(at: string | null | undefined): string {
	return at === null || at === undefined ? 'Never used' : formatCreated(new Date(at))
}

/**
 * Formats the moment a token ends.
 * @param at - The moment, or null when it never ends.
 * @param now - The moment to judge a lapsed token against.
 * @returns The date, or a plain statement of an endless or lapsed token.
 */
export function formatExpiry(at: string | null | undefined, now: Date): string {
	if (at === null || at === undefined) {
		return 'Never expires'
	}
	const ends = new Date(at)
	return ends.getTime() <= now.getTime() ? 'Expired' : formatCreated(ends)
}

/**
 * Returns the lifetime in days the given choice asks for.
 * @param choice - The chosen lifetime.
 * @returns The days, or null when the token never expires.
 */
export function lifetimeDays(choice: string): number | null {
	return choice === 'never' ? null : Number(choice)
}
