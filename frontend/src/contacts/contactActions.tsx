// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { RenameModal } from './ContactModals'
import type { ContactRow } from './contactFields'

/**
 * Returns the actions the contacts list offers on every row, the one click Add task first as the primary action.
 * @returns The row actions.
 */
export function useContactActions(): Action<ContactRow>[] {
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
				label: __('Rename…', 'alphone'),
				modalHeader: __('Rename', 'alphone'),
				modalFocusOnMount: 'firstContentElement',
				RenderModal: RenameModal,
			},
		],
		[navigate],
	)
}
