// SPDX-License-Identifier: AGPL-3.0-or-later

import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useSyncExternalStore } from 'react'
import { gql } from 'urql'
import type { Client, TypedDocumentNode } from 'urql'

import { useGraph } from './GraphProvider'
import { readFromNetwork } from './networkRead'

/** AdminSettings are the settings the admin screens read once, as the environment names them. */
export interface AdminSettings {
	/** toastMilliseconds is how long a toast with no button stays. */
	toastMilliseconds: number
	/** toastActionMilliseconds is how long a toast with a button stays. */
	toastActionMilliseconds: number
	/** dialogWriteTimeoutMilliseconds is how long a busy dialog waits on its write. */
	dialogWriteTimeoutMilliseconds: number
	/** listPageSizes are the page sizes a list offers. */
	listPageSizes: number[]
	/** listPageSize is the page size a list opens on. */
	listPageSize: number
	/** contactPageCap is the most contacts one contact page holds. */
	contactPageCap: number
	/** formatLocale is the locale dates, times, numbers and money are written in. */
	formatLocale: string
}

/** AdminSettingsRead is what the settings hook answers. */
export interface AdminSettingsRead {
	/** settings are the settings once the graph served them. */
	settings?: AdminSettings
	/** failed reports that the graph could not serve them. */
	failed: boolean
}

/** adminSettingsQuery reads the settings the admin screens read once. */
const adminSettingsQuery: TypedDocumentNode<{ adminSettings: AdminSettings }, Record<string, never>> = gql`
	query AdminSettings {
		adminSettings {
			toastMilliseconds
			toastActionMilliseconds
			dialogWriteTimeoutMilliseconds
			listPageSizes
			listPageSize
			contactPageCap
			formatLocale
		}
	}
`

/** The react-query key the admin settings are kept under. */
const adminSettingsKey = ['adminSettings'] as const

/**
 * Reads the admin settings from the network, failing when the graph could not serve them.
 * @param client - The graph client reading them.
 * @param signal - The signal that drops the read.
 * @returns The settings the graph served.
 */
async function readAdminSettings(client: Client, signal: AbortSignal): Promise<AdminSettings> {
	const answer = await readFromNetwork(client, adminSettingsQuery, signal)
	if (!answer.data) {
		throw answer.error
	}
	return answer.data.adminSettings
}

/**
 * Reads the admin settings once for each sign-in.
 * @returns The settings once served, beside whether the read failed.
 */
export function useAdminSettings(): AdminSettingsRead {
	const { client } = useGraph()
	const read = useQuery({
		queryKey: adminSettingsKey,
		queryFn: ({ signal }) => readAdminSettings(client, signal),
		staleTime: 'static',
		retry: false,
		retryOnMount: false,
	})
	return { settings: read.data, failed: read.isError }
}

/**
 * Returns the time limit a busy write gets, the one the admin settings serve once a screen loaded them.
 * @returns The time limit in milliseconds, Infinity while no screen loaded the settings.
 */
export function useWriteTimeout(): number {
	const client = useQueryClient()
	const subscribe = useCallback((changed: () => void) => client.getQueryCache().subscribe(changed), [client])
	return useSyncExternalStore(
		subscribe,
		() => client.getQueryData<AdminSettings>(adminSettingsKey)?.dialogWriteTimeoutMilliseconds ?? Infinity,
	)
}
