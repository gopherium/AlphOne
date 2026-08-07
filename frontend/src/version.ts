// SPDX-License-Identifier: AGPL-3.0-or-later

import { useGraphQuery } from '@alphone/frontend-sdk'

import { graphql } from './gql'

const versionQuery = graphql(`
	query Version {
		version
	}
`)

/**
 * Loads the application version from the graph.
 * @returns The version string, or undefined until it resolves.
 */
export function useAppVersion(): string | undefined {
	const [result] = useGraphQuery({ query: versionQuery })
	return result.data?.version
}
