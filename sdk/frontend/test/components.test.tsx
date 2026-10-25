// SPDX-License-Identifier: AGPL-3.0-or-later

import { fireEvent, render, screen } from '@testing-library/react'
import {
	DropZone as wordpressDropZone,
	FormTokenField as wordpressFormTokenField,
} from '@wordpress/components'
import { expect, test, vi } from 'vitest'

import { DropZone, FormTokenField } from '@alphone/frontend-sdk/components'

/** stylesheet records whether the WordPress components stylesheet was loaded. */
const stylesheet = vi.hoisted(() => ({ loaded: false }))

vi.mock('@wordpress/components/build-style/style.css', () => {
	stylesheet.loaded = true
	return {}
})

test('the components subpath loads the stylesheet its parts draw with', () => {
	expect(stylesheet.loaded).toBe(true)
})

test('hands plugins the WordPress drop zone and token field', () => {
	expect(DropZone).toBe(wordpressDropZone)
	expect(FormTokenField).toBe(wordpressFormTokenField)
})

test('FormTokenField shows each recipient as a chip with a button that removes it', () => {
	const onChange = vi.fn()
	render(<FormTokenField label="To" value={['maria.perez@example.com']} onChange={onChange} />)

	expect(screen.getByRole('combobox', { name: 'To' })).toBeInTheDocument()
	fireEvent.click(screen.getByRole('button', { name: 'Remove item', description: 'maria.perez@example.com (1 of 1)' }))

	expect(onChange).toHaveBeenCalledWith([])
})

test('DropZone shows its label and hands over the files dropped on it', () => {
	const onFilesDrop = vi.fn()
	const invoice = new File(['Invoice 184467235'], 'invoice.pdf', { type: 'application/pdf' })
	render(
		<div>
			<DropZone label="Drop files to attach" onFilesDrop={onFilesDrop} />
		</div>,
	)

	fireEvent.drop(screen.getByText('Drop files to attach'), {
		dataTransfer: { files: [invoice], items: [], types: ['Files'], getData: () => '' },
	})

	expect(onFilesDrop).toHaveBeenCalledWith([invoice])
})
