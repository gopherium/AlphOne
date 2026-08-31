// SPDX-License-Identifier: AGPL-3.0-or-later

/**
 * One change the schema diff detected.
 */
interface SchemaChange {
	path?: string
	criticality: { level: string }
}

/**
 * The schema removals this project has reviewed, by change path. A removal
 * belongs here once its consumers move in the same change and no client
 * outside this repository reads the field.
 */
const approved = new Set(['Mutation.createUser'])

/**
 * Answers the detected changes with every approved removal marked not breaking.
 * @param input - The changes the diff detected.
 * @returns The changes the diff reports.
 */
export default function approvedRemovals(input: { changes: SchemaChange[] }): SchemaChange[] {
	return input.changes.map((change) =>
		approved.has(change.path ?? '') && change.criticality.level === 'BREAKING'
			? { ...change, criticality: { ...change.criticality, level: 'NON_BREAKING' } }
			: change,
	)
}
