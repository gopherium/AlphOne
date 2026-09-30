// SPDX-License-Identifier: AGPL-3.0-or-later

import { Button, Stack, Text, __ } from '@alphone/frontend-sdk'
import { useId } from 'react'
import type { RefObject } from 'react'

/** ConfirmLabels are the words of a confirm group: its question and the button that answers yes. */
export interface ConfirmLabels {
	question: string
	confirm: string
}

/** ConfirmLayout is how a confirm group lays out: one wrapping line, or the question above a line of its buttons. */
export type ConfirmLayout = 'inline' | 'stacked'

/**
 * Renders a question that confirms a change, with the button that makes it and a Keep button.
 * @param props - The words, the buttons' state, the Keep ref, the actions and the layout.
 * @returns The confirm group.
 */
export function ConfirmActions({
	labels,
	locked,
	pending,
	keepRef,
	onConfirm,
	onKeep,
	layout = 'inline',
}: {
	labels: ConfirmLabels
	locked: boolean
	pending: boolean
	keepRef: RefObject<HTMLButtonElement | null>
	onConfirm: () => void
	onKeep: () => void
	layout?: ConfirmLayout
}) {
	const question = useId()
	const text = (
		<Text id={question} variant="body-sm">
			{labels.question}
		</Text>
	)
	const buttons = (
		<>
			<Button variant="minimal" tone="neutral" size="compact" disabled={locked} loading={pending} onClick={onConfirm}>
				{labels.confirm}
			</Button>
			<Button ref={keepRef} variant="minimal" tone="neutral" size="compact" disabled={locked} onClick={onKeep}>
				{__('Keep', 'alphone-fields')}
			</Button>
		</>
	)
	if (layout === 'stacked') {
		return (
			<Stack direction="column" gap="xs" align="start" role="group" aria-labelledby={question}>
				{text}
				<Stack direction="row" gap="xs" align="center" wrap="nowrap">
					{buttons}
				</Stack>
			</Stack>
		)
	}
	return (
		<Stack direction="row" gap="xs" align="center" wrap="wrap" role="group" aria-labelledby={question}>
			{text}
			{buttons}
		</Stack>
	)
}
