// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, __ } from '@alphone/frontend-sdk'
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
 * Returns the lifetime in days the given choice asks for.
 * @param choice - The chosen lifetime.
 * @returns The days, or null when the token never expires.
 */
export function lifetimeDays(choice: string): number | null {
	return choice === 'never' ? null : Number(choice)
}
