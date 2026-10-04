// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x, configureChannelNames } from '@alphone/frontend-sdk'

import { plugins } from '../plugins'

/** pluginChannels lists the channels the plugins name. */
const pluginChannels = plugins.flatMap((plugin) => plugin.channels ?? [])

/**
 * Returns the channel options, read fresh so the loaded catalogue answers.
 * @returns The options to offer.
 */
export function channelItems(): { value: string; label: string }[] {
	return [
		{ value: 'email', label: _x('Email', 'contact channel', 'alphone') },
		{ value: 'phone', label: _x('Phone', 'contact channel', 'alphone') },
	]
}

/**
 * Returns every channel the core and the plugins name, read fresh so the loaded catalogues answer.
 * @returns The channels, the core ones first.
 */
export function channelNames(): { value: string; label: string }[] {
	return [...channelItems(), ...pluginChannels]
}

/**
 * Hands the SDK the names the core and the plugins give each channel, so a plugin reads them as data.
 */
export function configureAppChannelNames(): void {
	configureChannelNames(channelNames)
}

/**
 * Reads the channel item a select item stands for.
 * @param item - The chosen item, or null when the selection is cleared.
 * @returns The matching channel item, or the email item as the default.
 */
export function channelItemOf(item: { value: string | null } | null) {
	const items = channelItems()
	return items.find((candidate) => candidate.value === item?.value) ?? items[0]
}
