// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	Card,
	ErrorNotice,
	LogList,
	SectionTitle,
	Stack,
	Text,
	__,
	sprintf,
	useGraph,
	useGraphMutation,
} from '@alphone/frontend-sdk'
import type { GraphFailure } from '@alphone/frontend-sdk'
import { useEffect, useId, useMemo, useRef, useState } from 'react'

import { EntryCells } from './cells'
import { draftCells, entriesOf, holdsInput, localDay, typedEntry } from './entries'
import type { RepeaterRow } from './entries'
import { entryMessage, outcomeOf, refetchAfter } from './entryOutcome'
import { EntryRow } from './EntryRow'
import { focusFirstControl } from './focus'
import { addContactFieldEntryMutation } from './operations'
import { useEntryActions } from './useEntryActions'
import type { GoneHandler } from './useEntryActions'

/**
 * Renders one repeater of a contact as a card: its heading in the header, one add form and its entries, newest first.
 * @param props - The contact, the repeater, its stored value and the report of a repeater gone.
 * @returns The repeater card.
 */
export function RepeaterEntries({
	contactId,
	field,
	stored,
	onGone,
}: {
	contactId: string
	field: RepeaterRow
	stored: unknown
	onGone: GoneHandler
}) {
	const heading = useId()
	const entries = useMemo(() => entriesOf(stored), [stored])
	const actions = useEntryActions({ contactId, field, entries, onGone })

	return (
		<Card.Root role="group" aria-labelledby={heading}>
			<Card.Header>
				<SectionTitle level={3} id={heading}>
					{field.label}
				</SectionTitle>
			</Card.Header>
			<Card.Content>
				<Stack direction="column" gap="sm">
					<AddEntryForm
						contactId={contactId}
						field={field}
						focusCount={actions.addFocus}
						onAdded={actions.added}
						onGone={onGone}
					/>
					{actions.failure !== '' && <ErrorNotice>{actions.failure}</ErrorNotice>}
					{entries.length === 0 ? (
						<Text role="status">{__('No entries yet.', 'alphone-fields')}</Text>
					) : (
						<LogList aria-labelledby={heading}>
							{entries.map((entry) => (
								<EntryRow
									key={entry.id}
									entry={entry}
									subFields={field.subFields}
									row={actions.rowOf(entry.id)}
									on={actions.on}
								/>
							))}
						</LogList>
					)}
				</Stack>
			</Card.Content>
		</Card.Root>
	)
}

/**
 * Renders the form adding one entry, its dates starting on today.
 * @param props - The contact, the repeater, the count of focus requests, and what runs on an add and a repeater gone.
 * @returns The add form.
 */
function AddEntryForm({
	contactId,
	field,
	focusCount,
	onAdded,
	onGone,
}: {
	contactId: string
	field: RepeaterRow
	focusCount: number
	onAdded: (from: Element | null) => void
	onGone: GoneHandler
}) {
	const [picked, setPicked] = useState<ReadonlyMap<string, string>>(() => new Map())
	const [add, runAdd] = useGraphMutation(addContactFieldEntryMutation)
	const graph = useGraph()
	const formRef = useRef<HTMLFormElement>(null)
	const shown = draftCells(field.subFields, picked, localDay(new Date()))
	const label = sprintf(__('Add an entry to %(label)s', 'alphone-fields'), { label: field.label })
	const fallback = __('The entry could not be added.', 'alphone-fields')

	useEffect(() => {
		if (focusCount > 0) {
			focusFirstControl(formRef.current as HTMLFormElement)
		}
	}, [focusCount])

	const submit = async () => {
		const from = document.activeElement
		const result = await runAdd({ contactId, field: field.name, entry: typedEntry(field.subFields, shown) })
		refetchAfter(graph, result.error)
		const outcome = outcomeOf(result.error)
		if (outcome === 'done') {
			setPicked(new Map())
			onAdded(from)
		} else if (outcome === 'field-gone') {
			onGone(entryMessage(result.error as GraphFailure, fallback), from)
		}
	}

	return (
		<form
			ref={formRef}
			className="godmin-form"
			aria-label={label}
			onSubmit={(event) => {
				event.preventDefault()
				void submit()
			}}
		>
			<EntryCells
				subFields={field.subFields}
				draft={shown}
				disabled={add.fetching}
				onChange={(name, text) => setPicked((held) => new Map(held).set(name, text))}
			/>
			{add.error && outcomeOf(add.error) !== 'field-gone' ? (
				<ErrorNotice>{entryMessage(add.error, fallback)}</ErrorNotice>
			) : null}
			<Button
				type="submit"
				disabled={!holdsInput(field.subFields, picked) || add.fetching}
				loading={add.fetching}
			>
				{label}
			</Button>
		</form>
	)
}
