// SPDX-License-Identifier: AGPL-3.0-or-later

import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { useState } from 'react'
import { expect, test, vi } from 'vitest'

import { Checkbox } from '../index'

test('renders an unticked checkbox with its accessible name', () => {
	render(<Checkbox label="Complete" checked={false} onCheckedChange={vi.fn()} />)

	const box = screen.getByRole('checkbox', { name: 'Complete' })
	expect(box).toBeInTheDocument()
	expect(box).not.toBeChecked()
})

test('renders a ticked checkbox', () => {
	render(<Checkbox label="Complete" checked onCheckedChange={vi.fn()} />)

	expect(screen.getByRole('checkbox', { name: 'Complete' })).toBeChecked()
})

test('reports a tick to the caller', async () => {
	const onCheckedChange = vi.fn()
	render(<Checkbox label="Complete" checked={false} onCheckedChange={onCheckedChange} />)

	await userEvent.click(screen.getByRole('checkbox', { name: 'Complete' }))

	expect(onCheckedChange).toHaveBeenCalledWith(true, expect.anything())
})

test('reports an untick to the caller', async () => {
	const onCheckedChange = vi.fn()
	render(<Checkbox label="Reopen" checked onCheckedChange={onCheckedChange} />)

	await userEvent.click(screen.getByRole('checkbox', { name: 'Reopen' }))

	expect(onCheckedChange).toHaveBeenCalledWith(false, expect.anything())
})

test('drives its own state when the caller does not', async () => {
	render(<Checkbox label="Complete" />)

	await userEvent.click(screen.getByRole('checkbox', { name: 'Complete' }))

	expect(screen.getByRole('checkbox', { name: 'Complete' })).toBeChecked()
})

test('refuses interaction while disabled', async () => {
	const onCheckedChange = vi.fn()
	render(
		<Checkbox label="Complete" checked={false} disabled onCheckedChange={onCheckedChange} />,
	)

	await userEvent.click(screen.getByRole('checkbox', { name: 'Complete' }))

	expect(onCheckedChange).not.toHaveBeenCalled()
	expect(screen.getByRole('checkbox', { name: 'Complete' })).toHaveAttribute(
		'aria-disabled',
		'true',
	)
})

test('ticks from the keyboard', async () => {
	function Controlled() {
		const [checked, setChecked] = useState(false)
		return <Checkbox label="Complete" checked={checked} onCheckedChange={setChecked} />
	}
	render(<Controlled />)

	await userEvent.tab()
	await userEvent.keyboard(' ')

	expect(screen.getByRole('checkbox', { name: 'Complete' })).toBeChecked()
})

test('carries a caller class alongside its own', () => {
	render(<Checkbox label="Complete" className="task-check" />)

	expect(screen.getByRole('checkbox', { name: 'Complete' })).toHaveClass('task-check')
})
