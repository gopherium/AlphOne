// SPDX-License-Identifier: AGPL-3.0-or-later

/** BOOT_QUERY asks the graph for the reader's language and the install's legal pages. */
const BOOT_QUERY = 'query AppLocale { locale legalLinks { privacyUrl termsUrl } }'

/** DEFAULT_LOCALE is the language the sources are written in. */
export const DEFAULT_LOCALE = 'en-US'

/** LegalAddresses holds the legal pages an install publishes, each empty when unset. */
export interface LegalAddresses {
	privacyUrl: string
	termsUrl: string
}

/** BootAnswer is what the read before the first paint settles. */
export interface BootAnswer {
	locale: string
	legal: LegalAddresses
}

/** AnsweredLinks is the legal part of a boot answer, as sent. */
interface AnsweredLinks {
	privacyUrl?: unknown
	termsUrl?: unknown
}

/** AnsweredBoot is the data of a boot answer, as sent. */
interface AnsweredBoot {
	locale?: unknown
	legalLinks?: AnsweredLinks | null
}

/**
 * Returns an answered value when it is text, empty for anything else.
 * @param value - The value as sent.
 * @returns The text.
 */
function answeredText(value: unknown): string {
	return typeof value === 'string' ? value : ''
}

/**
 * Returns what a boot answer settles, the default locale and no addresses for what it leaves out.
 * @param answered - The answer as sent, null when none came.
 * @returns The settled answer.
 */
function readBoot(answered: unknown): BootAnswer {
	const data = (answered as { data?: AnsweredBoot } | null)?.data
	const links = data?.legalLinks
	return {
		locale: answeredText(data?.locale) || DEFAULT_LOCALE,
		legal: {
			privacyUrl: answeredText(links?.privacyUrl),
			termsUrl: answeredText(links?.termsUrl),
		},
	}
}

/**
 * Returns the locale and the legal pages the server resolves for the caller, the defaults when it cannot say.
 * @returns The boot answer.
 */
export async function fetchBoot(): Promise<BootAnswer> {
	try {
		const response = await fetch('/api/graphql', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			credentials: 'same-origin',
			body: JSON.stringify({ query: BOOT_QUERY }),
		})
		return readBoot(await response.json())
	} catch {
		return readBoot(null)
	}
}
