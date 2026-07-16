// SPDX-License-Identifier: AGPL-3.0-or-later

import { useEventStream } from '@alphone/frontend-sdk'

/**
 * useLiveUpdates refetches the plugin's queries whenever the server
 * reports a conversation change over the events stream.
 */
export function useLiveUpdates() {
	useEventStream('/api/plugins/whatsapp/events', { invalidateKeys: [['whatsapp']] })
}
