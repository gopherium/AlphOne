// SPDX-License-Identifier: AGPL-3.0-or-later

/** SubFieldRow is one sub field of a repeater, edited as a cell of every entry. */
export interface SubFieldRow {
	name: string
	label: string
	kind: string
}

/** EntryText is the text of one repeater entry, keyed by sub field name. */
export type EntryText = Record<string, string>

/**
 * Returns the value an entry holds under one of its own keys, never an inherited one.
 * @param entry - The stored cells, keyed by sub field name.
 * @param key - The sub field name to read.
 * @returns The value, or undefined when the entry holds no such key of its own.
 */
function own(entry: Record<string, unknown>, key: string) {
	return Object.hasOwn(entry, key) ? entry[key] : undefined
}

/**
 * Returns the text form of a stored value.
 * @param stored - The value the graph answered.
 * @returns The text the input renders.
 */
export function textOf(stored: unknown) {
	if (stored === null || stored === undefined) {
		return ''
	}
	return String(stored)
}

/**
 * Returns the text of one entry, one cell per sub field.
 * @param subFields - The sub fields the entry holds.
 * @param entry - The stored cells, keyed by sub field name.
 * @returns The text of every cell, empty where the entry holds none.
 */
export function entryText(subFields: SubFieldRow[], entry: Record<string, unknown>): EntryText {
	return Object.fromEntries(subFields.map((column) => [column.name, textOf(own(entry, column.name))]))
}

/**
 * Returns one typed value the graph accepts for the given kind.
 * @param kind - The kind the definition declares.
 * @param text - The text the operator typed.
 * @returns The typed value, or null when the text is blank.
 */
export function typedValue(kind: string, text: string) {
	if (text === '') {
		return null
	}
	if (kind === 'NUMBER') {
		return Number(text)
	}
	if (kind === 'BOOLEAN') {
		return text === 'true'
	}
	return text
}
