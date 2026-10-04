// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ChannelName } from './index'

/**
 * Answers no channel names, as the SDK does until the core hands its own over.
 * @returns No names.
 */
function noNames(): ChannelName[] {
	return []
}

/** names answers every channel name the core and the plugins give. */
let names: () => ChannelName[] = noNames

/**
 * Stores the reader of every channel name the core and the plugins give.
 * @param reader - Answers the channel names, read fresh so the loaded catalogues answer.
 */
export function configureChannelNames(reader: () => ChannelName[]): void {
	names = reader
}

/**
 * Returns the name the core or a plugin gives a channel, or the channel itself when nobody names it.
 * @param channel - The channel an identity belongs to.
 * @returns The channel name to show.
 */
export function channelName(channel: string): string {
	return names().find((item) => item.value === channel)?.label ?? channel
}
