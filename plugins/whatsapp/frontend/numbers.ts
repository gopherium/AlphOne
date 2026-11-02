// SPDX-License-Identifier: AGPL-3.0-or-later

import { useGraphQuery } from '@alphone/frontend-sdk'

import type { WhatsAppNumbersQuery } from './gql/graphql'
import { numbersQuery } from './operations'

/** ConnectedNumber is one number the workspace holds as the graph selects it. */
export type ConnectedNumber = WhatsAppNumbersQuery['whatsAppNumbers'][number]

/**
 * useConnectedNumbers reads the numbers the workspace holds.
 * @returns The numbers, oldest first, none until they load.
 */
export function useConnectedNumbers(): readonly ConnectedNumber[] {
	const [numbers] = useGraphQuery({ query: numbersQuery, requestPolicy: 'cache-and-network' })
	return numbers.data?.whatsAppNumbers ?? []
}
