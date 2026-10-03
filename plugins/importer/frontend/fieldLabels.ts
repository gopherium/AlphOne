// SPDX-License-Identifier: AGPL-3.0-or-later

import { _x } from '@alphone/frontend-sdk'

/**
 * Returns the label each contact field the importer always offers reads as, read fresh so the loaded catalogue answers.
 * @returns The field name and label pairs.
 */
function contactFieldLabels(): [string, string][] {
	return [
		['name', _x('Name', 'contact field', 'alphone-importer')],
		['email', _x('Email', 'contact field', 'alphone-importer')],
		['phone', _x('Phone', 'contact field', 'alphone-importer')],
	]
}

/**
 * Returns the label a reader sees for each field name, the machine name when no live field carries it.
 * @param fields - The live fields the import offers.
 * @returns The labeller.
 */
export function fieldLabeller(fields: readonly { name: string; label: string }[]): (name: string) => string {
	const labels = new Map([...fields.map((field) => [field.name, field.label] as const), ...contactFieldLabels()])
	return (name) => labels.get(name) ?? name
}
