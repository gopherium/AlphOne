// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphError, graphExtensions, validationMessage } from '@alphone/frontend-sdk'
import type { GraphClient, GraphFailure } from '@alphone/frontend-sdk'

import { valuesOperation } from './document'
import { catalogueOperation } from './operations'

/** Outcome is what an entry call's answer asks the screen to do. */
export type Outcome = 'done' | 'refused' | 'entry-gone' | 'field-gone'

/** SPOKEN are the reasons whose own words show, though their code is not a validation. */
const SPOKEN = new Set(['field_entry_not_found', 'contact_not_found'])

/** OUTCOMES maps the reasons that change the screen to what they change. */
const OUTCOMES = new Map<string, Outcome>([
	['field_entry_not_found', 'entry-gone'],
	['field_unknown', 'field-gone'],
])

/** REFETCHES maps the reasons that make the screen read again to the queries it reads. */
const REFETCHES = new Map<string, readonly string[]>([
	['field_entry_not_found', [valuesOperation]],
	['field_entries_full', [valuesOperation]],
	['field_unknown', [catalogueOperation]],
])

/**
 * Returns the reason a refused entry call answered with.
 * @param error - The failure the call answered with.
 * @returns The reason, or an empty string.
 */
function reasonOf(error: GraphFailure): string {
	const reason = graphExtensions(error).reason
	return typeof reason === 'string' ? reason : ''
}

/**
 * Returns what a reader is shown for a refused entry call.
 * @param error - The failure the call answered with.
 * @param fallback - The words shown when the server said nothing readable.
 * @returns The message.
 */
export function entryMessage(error: GraphFailure, fallback: string): string {
	const said = graphError(error) as Error
	return SPOKEN.has(reasonOf(error)) ? said.message : validationMessage(said, fallback)
}

/**
 * Returns what an entry call's answer asks the screen to do.
 * @param error - The failure the call answered with, if any.
 * @returns The outcome.
 */
export function outcomeOf(error: GraphFailure | undefined): Outcome {
	if (error === undefined) {
		return 'done'
	}
	return OUTCOMES.get(reasonOf(error)) ?? 'refused'
}

/**
 * Reads again what an entry call's answer changed.
 * @param graph - The graph client.
 * @param error - The failure the call answered with, if any.
 */
export function refetchAfter(graph: GraphClient, error: GraphFailure | undefined): void {
	const names = error === undefined ? [valuesOperation] : (REFETCHES.get(reasonOf(error)) ?? [])
	if (names.length > 0) {
		graph.refetch(names)
	}
}
