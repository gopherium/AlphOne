// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen, within } from '@testing-library/react'
import { expect, test } from 'vitest'

import { LogItem, LogList, LogTime } from '../index'

test('hands plugins a log list of dated items', () => {
	render(
		<>
			<h3 id="notes">Notes</h3>
			<LogList aria-labelledby="notes">
				<LogItem
					aria-label="Note from Sep 1"
					label={<LogTime dateTime="2026-09-01">Sep 1</LogTime>}
					body="First call"
				/>
			</LogList>
		</>,
	)

	const list = screen.getByRole('list', { name: 'Notes' })
	const item = within(list).getByRole('listitem', { name: 'Note from Sep 1' })
	expect(within(item).getByText('Sep 1').closest('time')).toHaveAttribute('datetime', '2026-09-01')
	expect(within(item).getByText('First call')).toBeInTheDocument()
})
