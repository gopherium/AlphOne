// SPDX-License-Identifier: AGPL-3.0-or-later

import { globCatalogs, startLocale } from '@gopherium/gottext'
import type { Catalog, CatalogEntry } from '@gopherium/gottext'
import { DOMAIN as BRICK_DOMAIN, catalogFor as brickCatalogFor } from '@gopherium/react-auth'

import type { FrontendPlugin } from '@alphone/frontend-sdk'

import { DEFAULT_LOCALE, fetchLocale } from './api'
import { plugins } from '../plugins'

/** DOMAIN is the text domain AlphOne's own strings answer under. */
export const DOMAIN = 'alphone'

/** own holds the catalogues built from the committed sources. */
const own = import.meta.glob<{ default: Catalog }>('../languages/*.json')

/**
 * Returns the catalogue entry each plugin declares, skipping the ones declaring none.
 * @param registered - The plugins the build wired in.
 * @returns The declared entries, in registration order.
 */
export function declaredEntries(registered: FrontendPlugin[]): CatalogEntry[] {
	return registered.flatMap((plugin) => (plugin.locale === undefined ? [] : [plugin.locale]))
}

/**
 * Returns one catalogue entry per text domain the interface reads.
 * @returns The entries, AlphOne's own first and the auth brick's last.
 */
export function localeEntries(): CatalogEntry[] {
	return [
		{ domain: DOMAIN, load: globCatalogs(own) },
		...declaredEntries(plugins),
		{ domain: BRICK_DOMAIN, load: brickCatalogFor },
	]
}

/**
 * Settles the locale the interface stands in and loads the catalogues it reads.
 * @returns The settled locale.
 */
export async function startAppLocale(): Promise<string> {
	return startLocale(fetchLocale, localeEntries(), { defaultLocale: DEFAULT_LOCALE })
}
