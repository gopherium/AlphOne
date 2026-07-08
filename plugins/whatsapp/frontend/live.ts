// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQueryClient } from '@tanstack/react-query'
import { useEffect } from 'react'

/**
 * useLiveUpdates refetches the plugin's queries whenever the server
 * reports a conversation change over the events stream.
 */
export function useLiveUpdates() {
	const queryClient = useQueryClient()
	useEffect(() => {
		const source = new EventSource('/api/plugins/whatsapp/events')
		source.onmessage = () => {
			void queryClient.invalidateQueries({ queryKey: ['whatsapp'] })
		}
		return () => source.close()
	}, [queryClient])
}
