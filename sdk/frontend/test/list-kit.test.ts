// SPDX-License-Identifier: AGPL-3.0-or-later

import * as godmin from '@gopherium/godmin'
import * as godminRouter from '@gopherium/godmin/router'
import { expect, test } from 'vitest'

import {
	ConfirmBody,
	DrilldownItem,
	FormBody,
	ListEmpty,
	markLatest,
	MenuCount,
	NavigationItem,
	NavigationList,
	NavigationScreen,
	RenameBody,
	RunConfirm,
	useBulkRun,
	useListRun,
	useToaster,
	useUnsavedText,
} from '../index'
import type {
	BulkRun,
	BulkWords,
	ConfirmBodyProps,
	FormBodyProps,
	FormKind,
	ListRun,
	ListRunState,
	RenameBodyProps,
	RunConfirmProps,
	RunSettled,
	ToasterHandle,
	UnsavedText,
} from '../index'

/** The words a plugin's bulk run reports its outcome in, typed through the SDK. */
const WORDS: BulkWords<{ id: string }> = {
	done: (count) => `Done: ${count}.`,
	failed: (failures, asked) => `Failed: ${failures.length} of ${asked}.`,
}

/**
 * Expects the SDK to hand out godmin's own functions, each one present.
 * @param handed - The functions the SDK hands out.
 * @param shipped - The functions godmin ships under the same names.
 */
function expectGodminOwn(handed: unknown[], shipped: unknown[]) {
	expect(handed).toEqual(shipped)
	expect(handed.map((piece) => typeof piece)).toEqual(handed.map(() => 'function'))
}

test('hands plugins the list kit the admin runs, the very functions godmin ships', () => {
	expectGodminOwn([RunConfirm, useBulkRun, useListRun], [godmin.RunConfirm, godmin.useBulkRun, godmin.useListRun])
})

test('hands plugins the empty list the admin draws, the very one godmin ships', () => {
	expectGodminOwn([ListEmpty], [godmin.ListEmpty])
})

test('hands plugins the sidebar menu parts the admin draws, the very ones godmin ships', () => {
	expectGodminOwn(
		[NavigationScreen, MenuCount, NavigationList, NavigationItem, DrilldownItem],
		[
			godmin.NavigationScreen,
			godmin.MenuCount,
			godminRouter.NavigationList,
			godminRouter.NavigationItem,
			godminRouter.DrilldownItem,
		],
	)
})

test('hands plugins the dialog bodies the admin asks in, the very ones godmin ships', () => {
	expectGodminOwn([ConfirmBody, FormBody, RenameBody], [godmin.ConfirmBody, godmin.FormBody, godmin.RenameBody])
})

test('hands plugins the toaster the admin reports a failed write in, the very one godmin ships', () => {
	const handle: Pick<ToasterHandle, 'show'> | undefined = undefined

	expectGodminOwn([useToaster], [godmin.useToaster])
	expect(handle).toBeUndefined()
})

test('hands plugins the store that keeps what a failed write left unsaved, the very one godmin ships', () => {
	const unsaved: UnsavedText = new Map([['contact', 'Maria Perez']])

	expectGodminOwn([useUnsavedText, markLatest], [godmin.useUnsavedText, godmin.markLatest])
	expect(unsaved.get('contact')).toBe('Maria Perez')
})

test('hands plugins the types the list kit is written in', () => {
	const props: Pick<RunConfirmProps<{ id: string }>, 'confirmLabel' | 'cancelLabel'> = {
		confirmLabel: 'Revoke',
		cancelLabel: 'Cancel',
	}
	const run: BulkRun<{ id: string }> | undefined = undefined
	const list: ListRun | undefined = undefined
	const state: ListRunState | undefined = undefined
	const settled: RunSettled = () => undefined

	expect([props.confirmLabel, run, list, state, settled(true), WORDS.done(2, undefined)]).toEqual([
		'Revoke',
		undefined,
		undefined,
		undefined,
		undefined,
		'Done: 2.',
	])
})

test('hands plugins the types the dialog bodies are written in', () => {
	const kind: FormKind = 'copy'
	const confirm: Pick<ConfirmBodyProps, 'confirmLabel'> = { confirmLabel: 'Delete' }
	const form: Pick<FormBodyProps, 'submitLabel' | 'kind'> = { submitLabel: 'Duplicate', kind }
	const rename: Pick<RenameBodyProps, 'submitLabel'> = { submitLabel: 'Save' }

	expect([confirm.confirmLabel, form.submitLabel, form.kind, rename.submitLabel]).toEqual([
		'Delete',
		'Duplicate',
		'copy',
		'Save',
	])
})
