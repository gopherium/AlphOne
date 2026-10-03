// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'

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
 * Returns the name the core or a plugin gives a channel, or the channel itself when none does.
 * @param channel - The channel an identity belongs to.
 * @returns The channel name to show.
 */
export function channelName(channel: string): string {
	return channelNames().find((item) => item.value === channel)?.label ?? channel
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
