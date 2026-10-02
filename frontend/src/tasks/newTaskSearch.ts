// SPDX-License-Identifier: AGPL-3.0-or-later

/** NewTaskSearch is what the new task address carries. */
export interface NewTaskSearch {
	/** date is the day the task is due, as YYYY-MM-DD. */
	date?: string
	/** contactId names the contact the task starts linked to. */
	contactId?: string
}

/**
 * Keeps the day and the contact a new task address names, each only when it is text.
 * @param raw - The search the router parsed from the address.
 * @returns The new task search.
 */
export function newTaskSearch(raw: Record<string, unknown>): NewTaskSearch {
	return {
		...(typeof raw.date === 'string' ? { date: raw.date } : {}),
		...(typeof raw.contactId === 'string' ? { contactId: raw.contactId } : {}),
	}
}
