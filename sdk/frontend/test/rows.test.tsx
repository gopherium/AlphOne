// SPDX-License-Identifier: AGPL-3.0-or-later

import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'

import { RepeatRows, RowControls, TextareaControl, keyFromLabel } from '../index'
import type { RowLabels } from '../index'

/** labels are the words the notes editor shows. */
const labels: RowLabels = {
	add: 'Add entry',
	empty: 'No entries yet.',
	moveUp: 'Move entry up',
	moveDown: 'Move entry down',
	remove: 'Remove entry',
}

/** Holds a list of notes the SDK rows editor edits, each in a textarea. */
function Notes() {
	const [rows, setRows] = useState<string[]>([])
	return (
		<RepeatRows
			rows={rows}
			onChange={setRows}
			blank={() => ''}
			renderRow={(row, update, at) => (
				<TextareaControl
					label={`Note ${at + 1}`}
					value={row}
					onChange={(event) => update(event.target.value)}
				/>
			)}
			rowLabel={(at) => `Entry ${at + 1}`}
			labels={labels}
		/>
	)
}

test('hands plugins a rows editor whose rows edit in a textarea', () => {
	render(<Notes />)
	expect(screen.getByText('No entries yet.')).toBeInTheDocument()

	fireEvent.click(screen.getByRole('button', { name: 'Add entry' }))
	const note = screen.getByRole('textbox', { name: 'Note 1' })
	fireEvent.change(note, { target: { value: 'First call' } })

	expect(note.tagName).toBe('TEXTAREA')
	expect(screen.getByRole('textbox', { name: 'Note 1' })).toHaveValue('First call')
})

test('hands plugins the controls that move one row and take it away', () => {
	const moved = vi.fn()
	const removed = vi.fn()

	render(<RowControls at={0} count={2} labels={labels} onMove={moved} onRemove={removed} />)
	fireEvent.click(screen.getByRole('button', { name: 'Move entry down' }))
	fireEvent.click(screen.getByRole('button', { name: 'Remove entry' }))

	expect(screen.getByRole('button', { name: 'Move entry up' })).toHaveAttribute('aria-disabled', 'true')
	expect(moved).toHaveBeenCalledWith(1)
	expect(removed).toHaveBeenCalledTimes(1)
})

test('hands plugins a key made from a label', () => {
	expect(keyFromLabel('Birth date', { style: 'camel', taken: ['birthDate'] })).toBe('birthDate2')
})
