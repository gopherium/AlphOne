// SPDX-License-Identifier: AGPL-3.0-or-later

import { fireEvent, render, screen } from '@testing-library/react'
import { useState } from 'react'
import { expect, test } from 'vitest'

import { RepeatRows, TextareaControl, keyFromLabel } from '../index'

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
			labels={{
				add: 'Add entry',
				empty: 'No entries yet.',
				moveUp: 'Move entry up',
				moveDown: 'Move entry down',
				remove: 'Remove entry',
			}}
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

test('hands plugins a key made from a label', () => {
	expect(keyFromLabel('Birth date', { style: 'camel', taken: ['birthDate'] })).toBe('birthDate2')
})
