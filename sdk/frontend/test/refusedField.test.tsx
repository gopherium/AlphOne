// SPDX-License-Identifier: AGPL-3.0-or-later

import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useRef, useState } from 'react'
import { beforeEach, expect, test } from 'vitest'

import { Button, ValidatedInputControl, ValidationError, useRefusedField } from '../index'

const serverWords = 'Name is taken.'
const refusedName = 'Maria Perez'

/** The answer the last pressed Send got back from the hook. */
let answered: boolean | undefined

/**
 * Returns the region a screen reader hears assertive words from.
 * @returns The region.
 */
function spokenRegion() {
	return document.querySelector('#a11y-speak-assertive')
}

/** Renders a form whose Send refuses what was sent. */
function RefusableForm({
	sentAs = refusedName,
	failure = new ValidationError(serverWords),
}: {
	sentAs?: string
	failure?: Error
}) {
	const form = useRef<HTMLFormElement>(null)
	const [name, setName] = useState(refusedName)
	const refused = useRefusedField(form, name)
	return (
		<>
			<form ref={form} onSubmit={(event) => event.preventDefault()}>
				<ValidatedInputControl
					label="Name"
					value={name}
					customValidity={refused.validity}
					onChange={(event) => setName(event.target.value)}
				/>
			</form>
			<Button
				onClick={() => {
					answered = refused.refuse(failure, sentAs)
				}}
			>
				Send
			</Button>
		</>
	)
}

/** Renders a hook whose form never reached the page. */
function FormGone() {
	const form = useRef<HTMLFormElement>(null)
	const refused = useRefusedField(form, refusedName)
	return (
		<Button
			onClick={() => {
				answered = refused.refuse(new ValidationError(serverWords), refusedName)
			}}
		>
			Send
		</Button>
	)
}

beforeEach(() => {
	answered = undefined
	spokenRegion()?.replaceChildren()
})

test('shows the message under the field and moves the focus there', async () => {
	render(<RefusableForm />)

	fireEvent.click(screen.getByRole('button', { name: 'Send' }))

	const field = screen.getByRole('textbox', { name: 'Name' })
	await screen.findByText(serverWords)
	expect(answered).toBe(true)
	expect(field).toHaveAccessibleDescription(serverWords)
	expect(field).toHaveFocus()
})

test('takes the message away as the reader edits and holds it back from the value it is not about', async () => {
	render(<RefusableForm />)
	fireEvent.click(screen.getByRole('button', { name: 'Send' }))
	await screen.findByText(serverWords)

	fireEvent.change(screen.getByRole('textbox', { name: 'Name' }), { target: { value: 'Maria' } })

	await waitFor(() => expect(screen.queryByText(serverWords)).not.toBeInTheDocument())
	expect(screen.getByRole('textbox', { name: 'Name' })).toBeValid()
})

test('shows nothing when the reader types the refused value again, so it can be sent again', async () => {
	render(<RefusableForm />)
	fireEvent.click(screen.getByRole('button', { name: 'Send' }))
	await screen.findByText(serverWords)
	const field = screen.getByRole('textbox', { name: 'Name' })

	fireEvent.change(field, { target: { value: 'Maria' } })
	fireEvent.change(field, { target: { value: refusedName } })

	await waitFor(() => expect(screen.queryByText(serverWords)).not.toBeInTheDocument())
	expect(field).toBeValid()
})

test('answers no and shows nothing when the field has moved on from the value sent', () => {
	render(<RefusableForm sentAs="Another name" />)

	fireEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(answered).toBe(false)
	expect(screen.queryByText(serverWords)).not.toBeInTheDocument()
})

test('answers no when the failure carries no words worth showing', () => {
	render(<RefusableForm failure={new Error('boom')} />)

	fireEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(answered).toBe(false)
	expect(screen.getByRole('textbox', { name: 'Name' })).toBeValid()
})

test('answers no when the form has left the page', () => {
	render(<FormGone />)

	fireEvent.click(screen.getByRole('button', { name: 'Send' }))

	expect(answered).toBe(false)
})

test('speaks the message when the field already held the focus, as the focus move then says nothing', async () => {
	render(<RefusableForm />)
	screen.getByRole('textbox', { name: 'Name' }).focus()

	fireEvent.click(screen.getByRole('button', { name: 'Send' }))

	await waitFor(() => expect(spokenRegion()).toHaveTextContent(serverWords))
})

test('keeps quiet when the focus moves to the field, as the move itself reads the message out', async () => {
	render(<RefusableForm />)
	const send = screen.getByRole('button', { name: 'Send' })
	send.focus()

	fireEvent.click(send)

	await waitFor(() => expect(screen.getByRole('textbox', { name: 'Name' })).toHaveFocus())
	expect(spokenRegion()).toBeEmptyDOMElement()
})
