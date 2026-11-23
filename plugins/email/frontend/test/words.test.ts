// SPDX-License-Identifier: AGPL-3.0-or-later

import { expect, test } from 'vitest'

import { marksOf, previewOf, senderOf, subjectOf } from '../words'

/**
 * Builds the newest mail of a thread as the inbox reads it.
 * @param fields - The fields overriding a whole mail from Maria Perez with a preview.
 * @returns The mail.
 */
function mail(fields: Partial<Parameters<typeof previewOf>[0] & Parameters<typeof senderOf>[0]> = {}) {
	return {
		from: { name: 'Maria Perez', email: 'maria.perez@example.com' },
		snippet: 'Could you come by on Friday morning?',
		encrypted: false,
		signed: false,
		bodyState: 'whole',
		...fields,
	}
}

test('previews a mail by the text it starts with', () => {
	expect(previewOf(mail())).toBe('Could you come by on Friday morning?')
})

test('previews a mail cut at the body cap by the text it kept', () => {
	expect(previewOf(mail({ bodyState: 'cut' }))).toBe('Could you come by on Friday morning?')
})

test('previews a mail in a state it does not know by the text the server kept', () => {
	expect(previewOf(mail({ bodyState: 'partly_read' }))).toBe('Could you come by on Friday morning?')
})

test('previews an encrypted mail with fixed words, since it holds no text to show', () => {
	expect(previewOf(mail({ encrypted: true, snippet: '' }))).toBe('Encrypted message')
})

test('previews a mail read by its headers alone with fixed words', () => {
	expect(previewOf(mail({ bodyState: 'headers_only', snippet: '' }))).toBe('No text could be read')
})

test('says a mail is encrypted before it says the mail is signed', () => {
	expect(marksOf(mail({ encrypted: true, signed: true }))).toEqual(['Encrypted', 'Signed'])
})

test('marks a signed mail as signed alone', () => {
	expect(marksOf(mail({ signed: true }))).toEqual(['Signed'])
})

test('marks a plain mail with nothing', () => {
	expect(marksOf(mail())).toEqual([])
})

test('names a sender by the name the mail gives', () => {
	expect(senderOf(mail())).toBe('Maria Perez')
})

test('names a sender who gives no name by the address', () => {
	expect(senderOf(mail({ from: { name: '', email: 'maria.perez@example.com' } }))).toBe('maria.perez@example.com')
})

test('names no sender with fixed words when the mail names none', () => {
	expect(senderOf(mail({ from: null }))).toBe('(no sender)')
})

test('shows a thread by its subject', () => {
	expect(subjectOf({ subject: 'Delivery date' })).toBe('Delivery date')
})

test('shows a thread with no subject with fixed words', () => {
	expect(subjectOf({ subject: '' })).toBe('(no subject)')
})
