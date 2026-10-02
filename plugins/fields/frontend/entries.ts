// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, _x, formatDate, formatNumber, sprintf } from '@alphone/frontend-sdk'

import { entryText, typedValue } from './cellText'
import type { EntryText, SubFieldRow } from './cellText'
import { ENTRY_ID_KEY } from './kind'

/** RepeaterRow is one repeater of the catalogue with the sub fields its entries hold. */
export interface RepeaterRow {
	id: string
	name: string
	label: string
	subFields: SubFieldRow[]
}

/** StoredEntry is one entry the graph answered, under its id. */
export interface StoredEntry {
	id: string
	cells: Record<string, unknown>
}

/** EntryParts is what a row shows of one entry. */
interface EntryParts {
	day?: { at: string; text: string }
	body?: string
	lines: { key: string; text: string }[]
}

/** NUMBER_STYLE is how a row shows a number, to every decimal it holds. */
const NUMBER_STYLE: Intl.NumberFormatOptions = { maximumFractionDigits: 20 }

/** BODY_KINDS are the kinds whose cells make up a row's text. */
const BODY_KINDS = new Set(['TEXT', 'LONGTEXT'])

/**
 * Returns the entries a repeater's stored value holds, each under its id.
 * @param stored - The value the graph answered, absent when none are stored.
 * @returns The entries, in the order answered.
 */
export function entriesOf(stored: unknown): StoredEntry[] {
	return ((stored ?? []) as Record<string, unknown>[]).map((cells) => ({ id: String(cells[ENTRY_ID_KEY]), cells }))
}

/**
 * Returns the text an add form shows, the picked cells over today's date and blank text.
 * @param subFields - The sub fields an entry holds.
 * @param picked - The cells the operator changed.
 * @param today - The local calendar day date cells start on.
 * @returns The text of every cell.
 */
export function draftCells(subFields: SubFieldRow[], picked: ReadonlyMap<string, string>, today: string): EntryText {
	return Object.fromEntries(
		subFields.map((column) => [column.name, picked.get(column.name) ?? (column.kind === 'DATE' ? today : '')]),
	)
}

/**
 * Reports whether an add form holds something beyond the dates it starts with.
 * @param subFields - The sub fields an entry holds.
 * @param picked - The cells the operator changed.
 * @returns True when a changed cell or any other cell holds text.
 */
export function holdsInput(subFields: SubFieldRow[], picked: ReadonlyMap<string, string>): boolean {
	return subFields.some((column) => (picked.get(column.name) ?? '').trim() !== '')
}

/**
 * Reports whether every cell of an entry is blank.
 * @param text - The text of every cell.
 * @returns True when no cell holds more than white space.
 */
export function isBlank(text: EntryText): boolean {
	return Object.values(text).every((cell) => cell.trim() === '')
}

/**
 * Returns an entry in the form the entry mutations take.
 * @param subFields - The sub fields an entry holds.
 * @param text - The text of every cell.
 * @returns The typed cells, a blank cell as null.
 */
export function typedEntry(subFields: SubFieldRow[], text: EntryText): Record<string, unknown> {
	return Object.fromEntries(
		subFields.map((column) => {
			const cell = text[column.name]
			return [column.name, cell.trim() === '' ? null : typedValue(column.kind, cell)]
		}),
	)
}

/**
 * Returns what a row shows of one entry: its first date, its text and a line per other cell.
 * @param subFields - The sub fields the entry holds.
 * @param cells - The stored cells, keyed by sub field name.
 * @returns The parts of the row.
 */
export function entryParts(subFields: SubFieldRow[], cells: Record<string, unknown>): EntryParts {
	const text = entryText(subFields, cells)
	const filled = subFields.filter((column) => text[column.name].trim() !== '')
	const firstDate = subFields.find((column) => column.kind === 'DATE')
	const header = firstDate && filled.includes(firstDate) ? firstDate : undefined
	const said = filled.filter((column) => BODY_KINDS.has(column.kind)).map((column) => text[column.name])
	const rest = filled.filter((column) => column !== header && !BODY_KINDS.has(column.kind))
	return {
		day: header ? { at: text[header.name], text: dayText(text[header.name]) } : undefined,
		body: said.length > 0 ? said.join('\n\n') : undefined,
		lines: rest.map((column) => ({ key: column.name, text: detailLine(column, text[column.name]) })),
	}
}

/**
 * Returns the name a row is announced by: its day and its first words, else either one alone.
 * @param parts - The parts of the row.
 * @returns The name.
 */
export function entryName(parts: EntryParts): string {
	const said = firstWords(parts.body) ?? parts.lines[0]?.text
	if (parts.day === undefined || said === undefined) {
		return parts.day?.text ?? said ?? __('Blank entry', 'alphone-fields')
	}
	return sprintf(_x('%(day)s, %(text)s', 'entry name', 'alphone-fields'), { day: parts.day.text, text: said })
}

/**
 * Returns the first line of a text holding more than white space, trimmed.
 * @param body - The text, absent when the row holds none.
 * @returns The line, absent when there is no text.
 */
function firstWords(body: string | undefined): string | undefined {
	return body
		?.split('\n')
		.find((line) => line.trim() !== '')
		?.trim()
}

/**
 * Returns the entry focus moves to when one leaves, passing over entries that are leaving too.
 * @param entries - The entries shown, in order.
 * @param id - The entry leaving.
 * @param settling - The entries already leaving.
 * @returns The next entry's id, else the previous one's, else an empty string.
 */
export function neighbour(entries: readonly StoredEntry[], id: string, settling: readonly string[]): string {
	const open = entries.filter((entry) => entry.id === id || !settling.includes(entry.id))
	const at = open.findIndex((entry) => entry.id === id)
	return (open[at + 1] ?? open[at - 1])?.id ?? ''
}

/**
 * Returns a moment's local calendar day written YYYY-MM-DD.
 * @param at - The moment.
 * @returns The local calendar day.
 */
export function localDay(at: Date): string {
	const year = String(at.getFullYear()).padStart(4, '0')
	const month = String(at.getMonth() + 1).padStart(2, '0')
	const day = String(at.getDate()).padStart(2, '0')
	return `${year}-${month}-${day}`
}

/**
 * Returns one cell as a label and value line.
 * @param column - The sub field the cell belongs to.
 * @param text - The cell's text.
 * @returns The line.
 */
function detailLine(column: SubFieldRow, text: string): string {
	return sprintf(__('%(label)s: %(value)s', 'alphone-fields'), {
		label: column.label,
		value: cellValue(column.kind, text),
	})
}

/**
 * Returns a cell's text as a reader sees it.
 * @param kind - The kind the sub field declares.
 * @param text - The cell's text.
 * @returns Yes or No for a boolean, a shown day for a date, a shown number for a number, the text otherwise.
 */
function cellValue(kind: string, text: string): string {
	if (kind === 'BOOLEAN') {
		return text === 'true' ? _x('Yes', 'entry cell', 'alphone-fields') : _x('No', 'entry cell', 'alphone-fields')
	}
	if (kind === 'DATE') {
		return dayText(text)
	}
	if (kind === 'NUMBER') {
		return formatNumber(Number(text), NUMBER_STYLE)
	}
	return text
}

/**
 * Returns a calendar day as a reader sees it, on the day it names in every time zone.
 * @param day - The day written YYYY-MM-DD.
 * @returns The shown day.
 */
function dayText(day: string): string {
	return formatDate(day)
}
