// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useGraph, useGraphMutation, useToaster, writeFailure } from '@alphone/frontend-sdk'
import type { ToastAction } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { laterDate, movedMessage, shiftDate, statusMessage } from './format'
import { taskListReads, updateTaskMutation } from './operations'
import type { ListedTask, RowControls } from './TaskList'

/**
 * Returns the controls completing, reopening and postponing the rows of a task list, each confirmed with an Undo toast.
 * @param today - The current date as YYYY-MM-DD.
 * @returns The row controls.
 */
export function useRowControls(today: string): RowControls {
	const graph = useGraph()
	const toaster = useToaster()
	const [pendingID, setPendingID] = useState('')
	const [, runUpdate] = useGraphMutation(updateTaskMutation)
	const settled = ({ data, error }: { data?: unknown; error?: unknown }) => {
		if (data) {
			graph.refetch(taskListReads)
			return true
		}
		toaster.show(writeFailure(__('Could not update the task.', 'alphone'), error))
		return false
	}
	const setStatus = async (id: string, status: string) => {
		setPendingID(id)
		const result = await runUpdate({ id, input: { status } })
		setPendingID('')
		return settled(result)
	}
	const setDue = async (id: string, dueOn: string) => settled(await runUpdate({ id, input: { dueOn } }))
	const undo = (run: () => Promise<boolean>): ToastAction => ({ label: __('Undo', 'alphone'), onAct: () => void run() })
	const changeStatus = async (task: ListedTask) => {
		const status = task.status === 'done' ? 'open' : 'done'
		if (await setStatus(task.id, status)) {
			toaster.show(statusMessage(status), undo(() => setStatus(task.id, task.status)))
		}
	}
	const pushTask = async (task: ListedTask) => {
		const dueOn = shiftDate(laterDate(task.dueOn, today), 1)
		if (await setDue(task.id, dueOn)) {
			toaster.show(movedMessage(dueOn), undo(() => setDue(task.id, task.dueOn)))
		}
	}
	return {
		onChange: (task) => void changeStatus(task),
		onPush: (task) => void pushTask(task),
		pendingID,
	}
}
