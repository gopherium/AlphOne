// SPDX-License-Identifier: AGPL-3.0-or-later

import { globCatalogs, startLocale } from '@gopherium/gottext'
import type { Catalog } from '@gopherium/gottext'

import { DEFAULT_LOCALE, fetchLocale } from './api'

/** DOMAIN is the text domain AlphOne's own strings answer under. */
export const DOMAIN = 'alphone'

/** own holds the catalogues built from the committed sources. */
const own = import.meta.glob<{ default: Catalog }>('../languages/*.json')

/**
 * Settles the locale the interface stands in and loads the catalogues it reads.
 * @returns The settled locale.
 */
export async function startAppLocale(): Promise<string> {
	return startLocale(
		fetchLocale,
		[{ domain: DOMAIN, load: globCatalogs(own) }],
		{ defaultLocale: DEFAULT_LOCALE },
	)
}
