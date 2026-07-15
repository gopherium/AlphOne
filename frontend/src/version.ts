// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQuery } from '@tanstack/react-query'
import { z } from 'zod'

export const versionQueryKey = ['version'] as const

const versionSchema = z.object({ version: z.string() })

/**
 * Returns the version reported by the backend.
 * @returns The application version string.
 */
export async function fetchVersion(): Promise<string> {
	const response = await fetch('/api/version')
	if (!response.ok) {
		throw new Error(`loading version failed with status ${response.status}`)
	}
	return versionSchema.parse(await response.json()).version
}

/**
 * Loads the application version as a react-query result.
 * @returns The version query, whose data is the version string.
 */
export function useAppVersion() {
	return useQuery({ queryKey: versionQueryKey, queryFn: fetchVersion })
}
