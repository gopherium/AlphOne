// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	EmptyState,
	ErrorNotice,
	InputControl,
	LoadingRows,
	PageScreen,
	RepeatRows,
	SelectControl,
	Stack,
	Text,
	__,
	displayLocale,
	graphError,
	keyFromLabel,
	sprintf,
	useGraph,
	useGraphMutation,
	useGraphQuery,
	validationMessage,
} from '@alphone/frontend-sdk'
import { useState } from 'react'

import { fieldsIcon } from './icon'
import type { FieldKind } from './gql/graphql'
import { kindItems, kindOf, subKindItems } from './kind'
import { archiveFieldMutation, defineFieldMutation, fieldsQuery } from './operations'

const catalogueOperation = 'Fields'

/** FieldRow is one catalogue entry as the screen renders it. */
interface FieldRow {
	id: string
	name: string
	label: string
	kind: string
	subFields: SubFieldRow[]
}

/** SubFieldRow is one sub field of a repeater as the screen renders it. */
interface SubFieldRow {
	name: string
	label: string
	kind: string
}

/** DraftSubField is one sub field the add form holds before it is named. */
interface DraftSubField {
	label: string
	kind: FieldKind
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
			<PageScreen title={__('Fields', 'alphone-fields')}>
				<LoadingRows label={__('Loading fields…', 'alphone-fields')} rows={3} />
			</PageScreen>
		)
	}
	if (catalogue.error) {
		return (
			<PageScreen title={__('Fields', 'alphone-fields')}>
				<ErrorNotice>{__('The fields could not be loaded.', 'alphone-fields')}</ErrorNotice>
			</PageScreen>
		)
	}
	const fields = (catalogue.data?.fields ?? []) as FieldRow[]
	return (
		<PageScreen title={__('Fields', 'alphone-fields')}>
			<Stack direction="column" gap="lg">
				<FieldList fields={fields} onChanged={reload} />
				<Stack direction="column" gap="sm">
					<Text variant="heading-sm" render={<h2 />}>
						{__('Add a field', 'alphone-fields')}
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
				<EmptyState.Title>{__('No fields yet.', 'alphone-fields')}</EmptyState.Title>
				<EmptyState.Description>
					{__('Add a field to store more about every contact.', 'alphone-fields')}
				</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<Stack direction="column" gap="sm">
			{archived.error ? (
				<ErrorNotice>
					{validationMessage(graphError(archived.error), __('The field could not be archived.', 'alphone-fields'))}
				</ErrorNotice>
			) : null}
			<div
				className="godmin-table-scroll godmin-arrival"
				role="region"
				aria-label={__('Fields', 'alphone-fields')}
				tabIndex={0}
			>
				<table className="godmin-table">
					<thead>
						<tr>
							<th scope="col">{__('Label', 'alphone-fields')}</th>
							<th scope="col">{__('Name', 'alphone-fields')}</th>
							<th scope="col">{__('Kind', 'alphone-fields')}</th>
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
									<KindCell field={field} />
								</td>
								<td className="godmin-table__actions">
									<Button
										variant="outline"
										aria-label={sprintf(__('Archive %(label)s', 'alphone-fields'), { label: field.label })}
										loading={archived.fetching}
										onClick={() => {
											void archive({ id: field.id }).then(onChanged)
										}}
									>
										{__('Archive', 'alphone-fields')}
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
 * Renders the kind of one field, with the labels of its sub fields when it holds any.
 * @param props - The catalogue row.
 * @returns The kind cell content.
 */
function KindCell({ field }: { field: FieldRow }) {
	const badge = <Badge intent="stable">{kindLabel(field.kind)}</Badge>
	if (field.subFields.length === 0) {
		return badge
	}
	return (
		<Stack direction="column" gap="xs" align="start">
			{badge}
			<Text variant="body-sm">{labelList(field.subFields)}</Text>
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
 * Returns the labels of the given sub fields as one list in the reader's language.
 * @param subFields - The sub fields of a repeater, in order.
 * @returns The joined labels.
 */
function labelList(subFields: SubFieldRow[]) {
	return new Intl.ListFormat(displayLocale(), { type: 'unit' }).format(
		subFields.map((column) => column.label),
	)
}

/**
 * Returns the sub fields as the define mutation takes them, each named from its label.
 * @param drafts - The sub fields the operator listed, in order.
 * @returns The sub fields, each under a name no sibling repeats.
 */
function namedSubFields(drafts: DraftSubField[]) {
	const taken: string[] = []
	return drafts.map((draft) => {
		const name = keyFromLabel(draft.label, { style: 'camel', taken })
		taken.push(name)
		return { name, label: draft.label, kind: draft.kind }
	})
}

/**
 * Renders the form defining one new field.
 * @param props - The reload run after a definition lands.
 * @returns The add field form.
 */
function AddFieldForm({ onAdded }: { onAdded: () => void }) {
	const [label, setLabel] = useState('')
	const [name, setName] = useState('')
	const [kind, setKind] = useState<FieldKind>('TEXT')
	const [subFields, setSubFields] = useState<DraftSubField[]>([])
	const kinds = kindItems()
	const [defined, define] = useGraphMutation(defineFieldMutation)
	const repeater = kind === 'REPEATER'

	return (
		<form
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				const sent = repeater ? namedSubFields(subFields) : undefined
				void define({ name, label, kind, subFields: sent }).then((result) => {
					if (!result.error) {
						setLabel('')
						setName('')
						setSubFields([])
						onAdded()
					}
				})
			}}
		>
			{defined.error ? (
				<ErrorNotice>
					{validationMessage(graphError(defined.error), __('The field could not be defined.', 'alphone-fields'))}
				</ErrorNotice>
			) : null}
			<InputControl
				label={__('Label', 'alphone-fields')}
				autoComplete="off"
				value={label}
				onChange={(event) => setLabel(event.target.value)}
			/>
			<InputControl
				label={__('Name', 'alphone-fields')}
				autoComplete="off"
				value={name}
				onChange={(event) => setName(event.target.value)}
			/>
			<SelectControl
				label={__('Kind', 'alphone-fields')}
				items={kinds}
				value={kinds.find((option) => option.value === kind)}
				onValueChange={(item) => setKind(kindOf(item).value)}
			/>
			{repeater ? <SubFieldRows rows={subFields} onChange={setSubFields} /> : null}
			<Button type="submit" loading={defined.fetching}>
				{__('Add field', 'alphone-fields')}
			</Button>
		</form>
	)
}

/**
 * Renders the sub fields a new repeater holds, each with its label and kind.
 * @param props - The listed sub fields and what to call with a change.
 * @returns The sub fields editor.
 */
function SubFieldRows({
	rows,
	onChange,
}: {
	rows: DraftSubField[]
	onChange: (rows: DraftSubField[]) => void
}) {
	const kinds = subKindItems()

	return (
		<Stack direction="column" gap="sm">
			<Text variant="heading-sm" render={<h3 />}>
				{__('Sub fields', 'alphone-fields')}
			</Text>
			<RepeatRows
				rows={rows}
				onChange={onChange}
				blank={(): DraftSubField => ({ label: '', kind: 'TEXT' })}
				renderRow={(row, update) => (
					<>
						<InputControl
							label={__('Label', 'alphone-fields')}
							autoComplete="off"
							value={row.label}
							onChange={(event) => update({ ...row, label: event.target.value })}
						/>
						<SelectControl
							label={__('Kind', 'alphone-fields')}
							items={kinds}
							value={kinds.find((option) => option.value === row.kind)}
							onValueChange={(item) => update({ ...row, kind: kindOf(item).value })}
						/>
					</>
				)}
				rowLabel={(at) => sprintf(__('Sub field %(number)d', 'alphone-fields'), { number: at + 1 })}
				labels={{
					add: __('Add sub field', 'alphone-fields'),
					empty: __('No sub fields yet.', 'alphone-fields'),
					moveUp: __('Move sub field up', 'alphone-fields'),
					moveDown: __('Move sub field down', 'alphone-fields'),
					remove: __('Remove sub field', 'alphone-fields'),
				}}
			/>
		</Stack>
	)
}
