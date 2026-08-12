// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	InputControl,
	LoadingRows,
	PageScreen,
	SelectControl,
	Stack,
	Text,
	graphError,
	validationMessage,
	useGraphMutation,
	useGraphQuery,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { kindItems, kindOf } from './kind'
import { fieldsIcon } from './icon'
import { archiveFieldMutation, defineFieldMutation, fieldsQuery } from './operations'

/** FieldRow is one catalogue entry as the screen renders it. */
interface FieldRow {
	id: string
	name: string
	label: string
	kind: string
}

/**
 * Renders the catalogue of contact fields an operator defines.
 * @returns The fields screen.
 */
export function FieldsScreen() {
	const [catalogue, reload] = useGraphQuery({ query: fieldsQuery })

	if (catalogue.fetching && !catalogue.data) {
		return (
			<PageScreen title="Fields">
				<LoadingRows label="Loading fields…" rows={3} />
			</PageScreen>
		)
	}
	if (catalogue.error) {
		return (
			<PageScreen title="Fields">
				<ErrorNotice>The fields could not be loaded.</ErrorNotice>
			</PageScreen>
		)
	}
	const fields = (catalogue.data?.fields ?? []) as FieldRow[]
	return (
		<PageScreen title="Fields">
			<FieldList fields={fields} onChanged={reload} />
			<AddFieldForm onAdded={reload} />
		</PageScreen>
	)
}

/**
 * Renders the defined fields, or a placeholder when none exist.
 * @param props - The catalogue rows and the reload run after an archive.
 * @returns The field list.
 */
function FieldList({ fields, onChanged }: { fields: FieldRow[]; onChanged: () => void }) {
	const [archived, archive] = useGraphMutation(archiveFieldMutation)

	if (fields.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={fieldsIcon} />
				<EmptyState.Title>No fields yet.</EmptyState.Title>
				<EmptyState.Description>
					Add a field to store more about every contact.
				</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<>
			{archived.error ? (
				<ErrorNotice>
					{validationMessage(graphError(archived.error), 'The field could not be archived.')}
				</ErrorNotice>
			) : null}
			<ul className="alphone-fields__list">
				{fields.map((field) => (
					<li key={field.id}>
						<Text>{field.label}</Text>
						<Text>{field.name}</Text>
						<Text>{field.kind}</Text>
						<Button
							variant="outline"
							onClick={() => {
								void archive({ id: field.id }).then(onChanged)
							}}
						>
							{`Archive ${field.label}`}
						</Button>
					</li>
				))}
			</ul>
		</>
	)
}

/**
 * Renders the form defining one new field.
 * @param props - The reload run after a definition lands.
 * @returns The add field form.
 */
function AddFieldForm({ onAdded }: { onAdded: () => void }) {
	const [label, setLabel] = useState('')
	const [name, setName] = useState('')
	const [kind, setKind] = useState(kindItems[0])
	const [defined, define] = useGraphMutation(defineFieldMutation)

	return (
		<form
			onSubmit={(event) => {
				event.preventDefault()
				void define({ name, label, kind: kind.value }).then((result) => {
					if (!result.error) {
						setLabel('')
						setName('')
						onAdded()
					}
				})
			}}
		>
			<Stack>
				{defined.error ? (
					<ErrorNotice>
						{validationMessage(graphError(defined.error), 'The field could not be defined.')}
					</ErrorNotice>
				) : null}
				<InputControl
					label="Label"
					value={label}
					onChange={(event) => setLabel(event.target.value)}
				/>
				<InputControl
					label="Name"
					value={name}
					onChange={(event) => setName(event.target.value)}
				/>
				<SelectControl
					label="Kind"
					items={kindItems}
					value={kind}
					onValueChange={(item) => setKind(kindOf(item))}
				/>
				<Button type="submit">Add field</Button>
			</Stack>
		</form>
	)
}
