// SPDX-License-Identifier: AGPL-3.0-or-later

import { UnauthorizedError } from '@gopherium/react-auth'
import { z } from 'zod'

const contactSchema = z.object({
	id: z.string(),
	name: z.string(),
	created_at: z.coerce.date(),
})

export type Contact = z.infer<typeof contactSchema>

const identitySchema = z.object({
	channel: z.string(),
	identifier: z.string(),
	display_name: z.string(),
})

export type ContactIdentity = z.infer<typeof identitySchema>

const contactDetailSchema = contactSchema.extend({
	identities: z.array(identitySchema),
})

export type ContactDetail = z.infer<typeof contactDetailSchema>

const contactPageSchema = z.object({
	contacts: z.array(contactSchema),
	next_cursor: z.string().nullable(),
})

export type ContactPage = z.infer<typeof contactPageSchema>

/**
 * ValidationError is thrown when the backend rejects contact details.
 */
export class ValidationError extends Error {}

const errorSchema = z.object({ error: z.string() })

/**
 * Reads the backend's error message from a response, with a fallback.
 * @param response - The rejected response.
 * @param fallback - The message used when no error body is readable.
 * @returns The message to show.
 */
async function errorMessage(response: Response, fallback: string): Promise<string> {
	try {
		const parsed = errorSchema.safeParse(await response.json())
		return parsed.success ? parsed.data.error : fallback
	} catch {
		return fallback
	}
}

/**
 * Fetches one page of contacts, optionally filtered by a search query and
 * positioned by an opaque cursor.
 * @param query - The search query, or an empty string for all contacts.
 * @param cursor - The cursor from the previous page, or an empty string.
 * @returns The parsed page.
 */
export async function fetchContacts(query: string, cursor: string): Promise<ContactPage> {
	const params = new URLSearchParams()
	if (query !== '') {
		params.set('q', query)
	}
	if (cursor !== '') {
		params.set('cursor', cursor)
	}
	const encoded = params.toString()
	const response = await fetch(`/api/contacts${encoded === '' ? '' : `?${encoded}`}`)
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`listing contacts failed with status ${response.status}`)
	}
	return contactPageSchema.parse(await response.json())
}

/**
 * Fetches a contact with its identities.
 * @param id - The contact identifier.
 * @returns The parsed contact detail.
 */
export async function fetchContact(id: string): Promise<ContactDetail> {
	const response = await fetch(`/api/contacts/${id}`)
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (!response.ok) {
		throw new Error(`loading contact failed with status ${response.status}`)
	}
	return contactDetailSchema.parse(await response.json())
}

/**
 * Creates a contact and returns it.
 * @param name - The contact's name.
 * @returns The created contact.
 */
export async function createContact(name: string): Promise<Contact> {
	const response = await fetch('/api/contacts', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ name }),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, 'invalid contact details'))
	}
	if (!response.ok) {
		throw new Error(`creating contact failed with status ${response.status}`)
	}
	return contactSchema.parse(await response.json())
}

/**
 * Renames a contact and returns the updated contact.
 * @param id - The contact identifier.
 * @param name - The replacement name.
 * @returns The renamed contact.
 */
export async function renameContact(id: string, name: string): Promise<Contact> {
	const response = await fetch(`/api/contacts/${id}`, {
		method: 'PATCH',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ name }),
	})
	if (response.status === 401) {
		throw new UnauthorizedError('session expired')
	}
	if (response.status === 422) {
		throw new ValidationError(await errorMessage(response, 'invalid contact details'))
	}
	if (!response.ok) {
		throw new Error(`renaming contact failed with status ${response.status}`)
	}
	return contactSchema.parse(await response.json())
}
