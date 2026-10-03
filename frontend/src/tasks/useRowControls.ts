// SPDX-License-Identifier: AGPL-3.0-or-later

import { __, useGraph, useGraphMutation, useToaster } from '@alphone/frontend-sdk'
import type { ToastAction } from '@alphone/frontend-sdk'
import { useState } from 'react'

import { laterDate, movedMessage, shiftDate, statusMessage } from './format'
import { updateTaskMutation } from './operations'
import type { ListedTask, RowControls } from './TaskList'

/** ListControls are what the rows of a task list run and whether the last change or its undo failed. */
interface ListControls {
	controls: RowControls
	failed: boolean
}

/** listReads names the documents that list tasks, reread on whichever screen is open when a row change lands. */
const listReads = ['DayTasks', 'OverdueTasks', 'ContactDetail']

/**
 * Returns the controls completing, reopening and postponing the rows of a task list, each confirmed with an Undo toast.
 * @param today - The current date as YYYY-MM-DD.
 * @returns The row controls and whether the last change or its undo failed.
 */
export function useRowControls(today: string): ListControls {
	const graph = useGraph()
	const toaster = useToaster()
	const [pendingID, setPendingID] = useState('')
	const [change, runChange] = useGraphMutation(updateTaskMutation)
	const [push, runPush] = useGraphMutation(updateTaskMutation)
	const settled = (data: unknown) => {
		if (data) {
			graph.refetch(listReads)
		}
		return Boolean(data)
	}
	const setStatus = async (id: string, status: string) => {
		setPendingID(id)
		const result = await runChange({ id, input: { status } })
		setPendingID('')
		return settled(result.data)
	}
	const setDue = async (id: string, dueOn: string) => settled((await runPush({ id, input: { dueOn } })).data)
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
		controls: {
			onChange: (task) => void changeStatus(task),
			onPush: (task) => void pushTask(task),
			pendingID,
		},
		failed: Boolean(change.error || push.error),
	}
}
