// SPDX-License-Identifier: AGPL-3.0-or-later

import * as godmin from '@gopherium/godmin/dialogs'
import { expect, test } from 'vitest'

import { ConfirmDialog, FormDialog } from '../dialogs'
import type { ConfirmDialogProps, FormDialogProps } from '../dialogs'
import dialogs from '../dialogs.ts?raw'
import manifest from '../package.json'

/** The exports the SDK package names in its manifest. */
const exported: Record<string, string> = manifest.exports

test('hands plugins the dialogs the admin asks with outside a list, the very ones godmin ships', () => {
	expect([ConfirmDialog, FormDialog]).toEqual([godmin.ConfirmDialog, godmin.FormDialog])
	expect([typeof ConfirmDialog, typeof FormDialog]).toEqual(['function', 'function'])
})

test('hands plugins the types the dialogs are written in', () => {
	const confirm: Pick<ConfirmDialogProps, 'size' | 'confirmLabel'> = { size: 'small', confirmLabel: 'Revoke' }
	const form: Pick<FormDialogProps, 'size' | 'title'> = { size: 'medium', title: 'Add Invoice' }

	expect([confirm.size, confirm.confirmLabel, form.size, form.title]).toEqual([
		'small',
		'Revoke',
		'medium',
		'Add Invoice',
	])
})

test('serves the dialogs from their own entry, apart from the main one', () => {
	expect(exported['./dialogs']).toBe('./dialogs.ts')
})

test('loads the stylesheet of the components the dialogs draw with', () => {
	expect(dialogs).toContain('@wordpress/components/build-style/style.css')
})
