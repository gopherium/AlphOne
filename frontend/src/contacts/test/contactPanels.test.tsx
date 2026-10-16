// SPDX-License-Identifier: AGPL-3.0-or-later

import type { ContactPanel } from '@alphone/frontend-sdk'
import { render, screen } from '@testing-library/react'
import { lazy } from 'react'
import { expect, test } from 'vitest'

import { ContactPanels } from '../ContactPanels'

const contactID = '0198c000-0000-7000-8000-000000000401'

function panel(id: string): ContactPanel {
	return { id, Panel: ({ contactId }) => <p>{`${id} for ${contactId}`}</p> }
}

test('every contributed panel renders for the contact', () => {
	render(
		<ContactPanels contactId={contactID} panels={[panel('fields'), panel('notes')]} />,
	)

	expect(screen.getByText(`fields for ${contactID}`)).toBeInTheDocument()
	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
})

test('keeps every other panel on screen while one panel loads', () => {
	const loading: ContactPanel = { id: 'fields', Panel: lazy(() => new Promise<never>(() => {})) }

	render(<ContactPanels contactId={contactID} panels={[loading, panel('notes')]} />)

	expect(screen.getByText(`notes for ${contactID}`)).toBeInTheDocument()
})

test('no panels render nothing at all', () => {
	const { container } = render(<ContactPanels contactId={contactID} panels={[]} />)

	expect(container).toBeEmptyDOMElement()
})
