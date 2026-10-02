// SPDX-License-Identifier: AGPL-3.0-or-later

import { gql, useQuery } from 'urql'
import type { TypedDocumentNode } from 'urql'

import { useSession } from './session'

/** AdminSettings are the settings the admin screens read once, as the environment names them. */
export interface AdminSettings {
	/** toastMilliseconds is how long a confirmation toast stays. */
	toastMilliseconds: number
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
			listPageSizes
			listPageSize
			contactPageCap
			formatLocale
		}
	}
`

/**
 * Reads the admin settings once somebody is signed in.
 * @returns The settings once served, beside whether the read failed.
 */
export function useAdminSettings(): AdminSettingsRead {
	const session = useSession()
	const [result] = useQuery({ query: adminSettingsQuery, pause: session === null })
	return { settings: result.data?.adminSettings, failed: result.error !== undefined }
}
