// SPDX-License-Identifier: AGPL-3.0-or-later

/** LOCALE_QUERY asks the graph which language to serve the reader in. */
const LOCALE_QUERY = 'query AppLocale { locale }'

/** DEFAULT_LOCALE is the language the sources are written in. */
export const DEFAULT_LOCALE = 'en-US'

/**
 * Returns the locale the server resolves for the caller, the default when it cannot say.
 * @returns The locale to read in.
 */
export async function fetchLocale(): Promise<string> {
	try {
		const response = await fetch('/api/graphql', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			credentials: 'same-origin',
			body: JSON.stringify({ query: LOCALE_QUERY }),
		})
		const answered: unknown = await response.json()
		const locale = (answered as { data?: { locale?: unknown } })?.data?.locale
		return typeof locale === 'string' && locale !== '' ? locale : DEFAULT_LOCALE
	} catch {
		return DEFAULT_LOCALE
	}
}
