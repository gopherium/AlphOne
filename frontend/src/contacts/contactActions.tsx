// SPDX-License-Identifier: AGPL-3.0-or-later

import { __ } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { RenameModal } from './ContactModals'
import type { ContactRow } from './contactFields'

/**
 * Returns the actions the contacts list offers on every row, the modal first and the one click action last.
 * @returns The row actions.
 */
export function useContactActions(): Action<ContactRow>[] {
	const navigate = useNavigate()

	return useMemo(
		() => [
			{
				id: 'rename',
				label: __('Rename', 'alphone'),
				modalHeader: __('Rename contact', 'alphone'),
				modalSize: 'small',
				modalFocusOnMount: 'firstContentElement',
				RenderModal: RenameModal,
			},
			{
				id: 'add-task',
				label: __('Add task', 'alphone'),
				callback: ([contact]) => void navigate({ to: '/tasks/new', search: { contactId: contact.id } }),
			},
		],
		[navigate],
	)
}
