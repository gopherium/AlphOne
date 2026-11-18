// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import type { ContactRow } from './contactFields'

/**
 * Returns the actions the contacts list offers on every row, the one click Add task first as the primary action.
 * @param onRename - Stores the contact the reader asked to rename.
 * @returns The row actions.
 */
export function useContactActions(onRename: (contact: ContactRow) => void): Action<ContactRow>[] {
	const navigate = useNavigate()

	return useMemo(
		() => [
			{
				id: 'add-task',
				label: __('Add task', 'alphone'),
				isPrimary: true,
				callback: ([contact]) => void navigate({ to: '/tasks/new', search: { contactId: contact.id } }),
			},
			{
				id: 'rename',
				label: __('Rename', 'alphone'),
				callback: ([contact]) => onRename(contact),
			},
		],
		[navigate, onRename],
	)
}
