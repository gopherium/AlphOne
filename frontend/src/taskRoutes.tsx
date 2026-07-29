// SPDX-License-Identifier: AGPL-3.0-or-later

import { getRouteApi, useNavigate } from '@tanstack/react-router'

import { NewTaskScreen } from './tasks/NewTaskScreen'
import { TaskScreen } from './tasks/TaskScreen'
import { TasksScreen } from './tasks/TasksScreen'
import { isoDate, isValidDate } from './tasks/format'

const tasksRouteApi = getRouteApi('/tasks')
const newTaskRouteApi = getRouteApi('/tasks/new')
const taskRouteApi = getRouteApi('/tasks/$taskId')

/**
 * Returns the day to show, defaulting to today for a missing or unusable
 * date.
 * @param date - The date from the search params.
 * @param today - The current date as YYYY-MM-DD.
 * @returns The day to show as YYYY-MM-DD.
 */
function dayOrToday(date: string | undefined, today: string): string {
	return date !== undefined && isValidDate(date) ? date : today
}

/**
 * Renders the task list for the day in the search params, defaulting to
 * today.
 * @returns The tasks route element.
 */
export function TasksRoute() {
	const { date } = tasksRouteApi.useSearch()
	const today = isoDate(new Date())
	return <TasksScreen date={dayOrToday(date, today)} today={today} />
}

/**
 * Renders the new-task form, opening the created task's detail on success.
 * @returns The new task route element.
 */
export function NewTaskRoute() {
	const { date } = newTaskRouteApi.useSearch()
	const navigate = useNavigate()
	return (
		<NewTaskScreen
			date={dayOrToday(date, isoDate(new Date()))}
			onCreated={(created) =>
				void navigate({ to: '/tasks/$taskId', params: { taskId: created.id } })
			}
		/>
	)
}

/**
 * Renders the task detail screen for the route's task id.
 * @returns The task route element.
 */
export function TaskRoute() {
	const { taskId } = taskRouteApi.useParams()
	return <TaskScreen taskId={taskId} />
}
