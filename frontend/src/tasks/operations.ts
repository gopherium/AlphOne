// SPDX-License-Identifier: AGPL-3.0-or-later

import { graphql } from '../gql'

export const dayTasksQuery = graphql(`
	query DayTasks($date: Date!, $status: String!, $first: Int!, $after: String) {
		tasks(date: $date, status: $status, first: $first, after: $after) {
			edges {
				node {
					id
					title
					status
					priority
					dueOn
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}
`)

export const overdueTasksQuery = graphql(`
	query OverdueTasks($dueBefore: Date!, $status: String!, $first: Int!, $after: String) {
		tasks(dueBefore: $dueBefore, status: $status, first: $first, after: $after) {
			edges {
				node {
					id
					title
					status
					priority
					dueOn
				}
				cursor
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}
`)
