// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const supportedLocalesQuery = graphql(`
	query SupportedLocales {
		supportedLocales
	}
`)

export const setLocaleMutation = graphql(`
	mutation SetLocale($locale: String!) {
		setLocale(locale: $locale)
	}
`)
