// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
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
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { fieldsIcon } from './icon'
import { kindItems, kindOf } from './kind'
import { archiveFieldMutation, defineFieldMutation, fieldsQuery } from './operations'

const catalogueOperation = 'Fields'

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
	const [catalogue] = useGraphQuery({ query: fieldsQuery })
	const reload = useCatalogueRefresh()

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
			<Stack direction="column" gap="lg">
				<FieldList fields={fields} onChanged={reload} />
				<Stack direction="column" gap="sm">
					<Text variant="heading-sm" render={<h2 />}>
						Add a field
					</Text>
					<AddFieldForm onAdded={reload} />
				</Stack>
			</Stack>
		</PageScreen>
	)
}

/**
 * Returns the refresh rerunning the catalogue query against the network.
 * @returns The refresh callback.
 */
function useCatalogueRefresh() {
	const graph = useGraph()
	return () => {
		graph.refetch([catalogueOperation])
	}
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
		<Stack direction="column" gap="sm">
			{archived.error ? (
				<ErrorNotice>
					{validationMessage(graphError(archived.error), 'The field could not be archived.')}
				</ErrorNotice>
			) : null}
			<div
				className="godmin-table-scroll godmin-arrival"
				role="region"
				aria-label="Fields"
				tabIndex={0}
			>
				<table className="godmin-table">
					<thead>
						<tr>
							<th scope="col">Label</th>
							<th scope="col">Name</th>
							<th scope="col">Kind</th>
							<th scope="col" className="godmin-table__actions" />
						</tr>
					</thead>
					<tbody>
						{fields.map((field) => (
							<tr key={field.id}>
								<td>{field.label}</td>
								<td>
									<code>{field.name}</code>
								</td>
								<td>
									<Badge intent="stable">{kindLabel(field.kind)}</Badge>
								</td>
								<td className="godmin-table__actions">
									<Button
										variant="outline"
										aria-label={`Archive ${field.label}`}
										loading={archived.fetching}
										onClick={() => {
											void archive({ id: field.id }).then(onChanged)
										}}
									>
										Archive
									</Button>
								</td>
							</tr>
						))}
					</tbody>
				</table>
			</div>
		</Stack>
	)
}

/**
 * Returns the human label of one field kind.
 * @param kind - The kind the definition declares.
 * @returns The label shown in the catalogue.
 */
function kindLabel(kind: string) {
	return kindOf({ value: kind }).label
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
			className="godmin-form"
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
			{defined.error ? (
				<ErrorNotice>
					{validationMessage(graphError(defined.error), 'The field could not be defined.')}
				</ErrorNotice>
			) : null}
			<InputControl
				label="Label"
				autoComplete="off"
				value={label}
				onChange={(event) => setLabel(event.target.value)}
			/>
			<InputControl
				label="Name"
				autoComplete="off"
				value={name}
				onChange={(event) => setName(event.target.value)}
			/>
			<SelectControl
				label="Kind"
				items={kindItems}
				value={kind}
				onValueChange={(item) => setKind(kindOf(item))}
			/>
			<Button type="submit" loading={defined.fetching}>
				Add field
			</Button>
		</form>
	)
}
