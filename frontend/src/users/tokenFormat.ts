// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, __, formatDate } from '@alphone/frontend-sdk'
import type { FrontendPlugin, TokenArea } from '@alphone/frontend-sdk'

import { plugins as wired } from '../plugins'

/**
 * Returns the areas the core grants a token, each named the way it reads in a sentence, read fresh.
 * @returns The core areas.
 */
function coreAreas(): TokenArea[] {
	return [
		{ value: 'contacts', label: _x('contacts', 'token area', 'alphone') },
		{ value: 'events', label: _x('events', 'token area', 'alphone') },
		{ value: 'meta', label: _x('meta', 'token area', 'alphone') },
		{ value: 'tasks', label: _x('tasks', 'token area', 'alphone') },
		{ value: 'users', label: _x('users', 'token area', 'alphone') },
		{ value: 'webhooks', label: _x('webhooks', 'token area', 'alphone') },
	]
}

/**
 * Returns every area a token may be granted, the core ones and each plugin's, in order of their names.
 * @param plugins - The plugins whose areas join the core ones, the wired ones by default.
 * @returns The areas, refusing one granted twice.
 */
export function grantableAreas(plugins: readonly Pick<FrontendPlugin, 'id' | 'tokenAreas'>[] = wired): TokenArea[] {
	const areas = coreAreas()
	const owners = new Map(areas.map((area) => [area.value, 'the core']))
	for (const plugin of plugins) {
		for (const area of plugin.tokenAreas ?? []) {
			const owner = owners.get(area.value)
			if (owner !== undefined) {
				throw new Error(`tokens: ${plugin.id} grants the area ${area.value}, which ${owner} already grants`)
			}
			owners.set(area.value, plugin.id)
			areas.push(area)
		}
	}
	return areas.sort((left, right) => (left.value < right.value ? -1 : 1))
}

/**
 * Returns the lifetimes the mint form offers, read fresh so the loaded catalogue answers.
 * @returns The choices, in menu order.
 */
export function lifetimeChoices(): { value: string; label: string }[] {
	return [
		{ value: '30', label: __('30 days', 'alphone') },
		{ value: '90', label: __('90 days', 'alphone') },
		{ value: '365', label: __('A year', 'alphone') },
		{ value: 'never', label: _x('Never', 'token lifetime', 'alphone') },
	]
}

/** defaultLifetime is the lifetime the mint form starts on. */
export const defaultLifetime = '90'

/**
 * Formats the moment a token was last presented.
 * @param at - The moment, or null when it never acted.
 * @returns The date, or a plain statement that it never acted.
 */
export function formatLastUsed(at: string | null | undefined): string {
	return at === null || at === undefined ? __('Never used', 'alphone') : formatDate(at)
}

/**
 * Formats the moment a token ends.
 * @param at - The moment, or null when it never ends.
 * @param now - The moment to judge a lapsed token against.
 * @returns The date, or a plain statement of an endless or lapsed token.
 */
export function formatExpiry(at: string | null | undefined, now: Date): string {
	if (at === null || at === undefined) {
		return __('Never expires', 'alphone')
	}
	const ends = new Date(at)
	return ends.getTime() <= now.getTime() ? _x('Expired', 'token expiry', 'alphone') : formatDate(ends)
}

/**
 * Returns the lifetime in days the given choice asks for.
 * @param choice - The chosen lifetime.
 * @returns The days, or null when the token never expires.
 */
export function lifetimeDays(choice: string): number | null {
	return choice === 'never' ? null : Number(choice)
}
