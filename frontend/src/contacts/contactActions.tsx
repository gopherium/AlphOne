// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useUnsavedText } from '@alphone/frontend-sdk'
import type { ListRun } from '@alphone/frontend-sdk'
import type { Action } from '@alphone/frontend-sdk/dataviews'
import { useNavigate } from '@tanstack/react-router'
import { useMemo } from 'react'

import { RenameModal } from './ContactModals'
import type { ContactRow } from './contactFields'

/**
 * Returns the actions the contacts list offers on every row, the one click Add task first as the primary action.
 * @param run - What the list does around a write of its own.
 * @param reload - Reads the page on show again, the same function while the screen stays.
 * @returns The row actions.
 */
export function useContactActions(run: ListRun, reload: () => Promise<unknown>): Action<ContactRow>[] {
	const navigate = useNavigate()
	const renames = useUnsavedText()

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
				RenderModal: (props) => <RenameModal {...props} unsaved={renames} run={run} reload={reload} />,
			},
		],
		[navigate, renames, run, reload],
	)
}
