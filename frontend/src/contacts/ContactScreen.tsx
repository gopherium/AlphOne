// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	ErrorNotice,
	IconButton,
	InputControl,
	LoadingScreen,
	LogItem,
	LogList,
	PageBreadcrumb,
	PageBreadcrumbs,
	PageScreen,
	SectionTitle,
	SelectControl,
	Text,
	ValidatedInputControl,
	ValidationError,
	__,
	channelName,
	formatDate,
	graphError,
	sprintf,
	graphExtensions,
	trash,
	useConnection,
	useGraph,
	useGraphMutation,
	useRefusedField,
	useToaster,
	writeFailure,
} from '@alphone/frontend-sdk'
import type { ConnectionResult, GraphFailure } from '@alphone/frontend-sdk'
import { Link, useBlocker } from '@tanstack/react-router'
import { useId, useRef, useState } from 'react'
import type { ComponentProps, ReactNode, RefObject } from 'react'

import { ContactTasks } from '../tasks/ContactTasks'
import type { ListedTask } from '../tasks/TaskList'
import { ContactPanels } from './ContactPanels'
import { channelItemOf, channelItems } from './channel'
import {
	addContactIdentityMutation,
	contactDetailQuery,
	deleteContactIdentityMutation,
	renameContactMutation,
} from './operations'
import { contactPanels } from './panels'

const contactTasksPageSize = 10
const contactDetailOperation = 'ContactDetail'

/** The reasons an added identity fails for that are about its channel, not its value. */
const channelReasons: readonly unknown[] = ['identity_channel_required', 'channel_not_writable']

/** ContactDetail is the contact the screen renders, identities included. */
export interface ContactDetail {
	id: string
	name: string
	createdAt: string
	identities: {
		id: string
		channel: string
		identifier: string
		displayName: string
	}[]
}

/**
 * Renders the links that lead from a contact page back to the contacts.
 * @returns The breadcrumbs.
 */
export function ContactBreadcrumbs() {
	return (
		<PageBreadcrumbs label={__('Breadcrumbs', 'alphone')}>
			<PageBreadcrumb render={<Link to="/contacts" activeOptions={{ exact: true }} />}>
				{__('Contacts', 'alphone')}
			</PageBreadcrumb>
		</PageBreadcrumbs>
	)
}

/**
 * Renders one contact's page, or what stands in for it while it loads or after it failed to load.
 * @returns The contact screen.
 */
export function ContactScreen({ contactId }: { contactId: string }) {
	const detail = useConnection({
		query: contactDetailQuery,
		variables: { id: contactId, first: contactTasksPageSize },
		select: (data) => data.contact?.tasks,
	})
	const contact = detail.data?.contact

	if (detail.isPending) {
		return (
			<UnloadedContact>
				<LoadingScreen label={__('Loading contact…', 'alphone')} />
			</UnloadedContact>
		)
	}
	if (!contact) {
		return (
			<UnloadedContact>
				<ErrorNotice>{__('The contact could not be loaded.', 'alphone')}</ErrorNotice>
			</UnloadedContact>
		)
	}
	return <ContactRecord key={contact.id} contact={contact} tasks={detail} />
}

/**
 * Renders the page of a contact nothing is known of, titled by the kind of record, with no details column.
 * @param props - What the page shows.
 * @returns The page.
 */
function UnloadedContact({ children }: { children: ReactNode }) {
	return (
		<PageScreen title={__('Contact', 'alphone')} headingLevel={2} breadcrumbs={<ContactBreadcrumbs />}>
			{children}
		</PageScreen>
	)
}

/**
 * Renders a loaded contact: its tasks and plugin panels as the main work, its name, identities and creation date as
 * the details, and the Save button of its name in the page header.
 * @param props - The contact and its tasks.
 * @returns The contact's page.
 */
function ContactRecord({ contact, tasks }: { contact: ContactDetail; tasks: ConnectionResult<ListedTask> }) {
	const settled = useContactRefresh()
	const toaster = useToaster()
	const identitiesHeading = useId()
	const nameForm = useId()
	const form = useRef<HTMLFormElement>(null)
	const [draft, setDraft] = useState<string | null>(null)
	const [rename, runRename] = useGraphMutation(renameContactMutation)
	const name = draft ?? contact.name
	const refusedName = useRefusedField(form, name)
	const changed = draft !== null && draft !== contact.name
	const blank = name.trim() === ''
	useBlocker({
		shouldBlockFn: () => !window.confirm(__('Leave this page? The name is not saved.', 'alphone')),
		disabled: !changed,
	})
	const submitRename = async () => {
		const result = await runRename({ id: contact.id, name })
		if (result.data) {
			setDraft(null)
			toaster.show(__('Name updated.', 'alphone'))
			settled()
		} else if (!refusedName.refuse(result.error, name)) {
			toaster.show(writeFailure(__('Could not save the contact.', 'alphone'), result.error))
		}
	}

	return (
		<PageScreen
			title={blank ? __('Contact', 'alphone') : name}
			headingLevel={2}
			breadcrumbs={<ContactBreadcrumbs />}
			actions={
				<Button
					type="submit"
					form={nameForm}
					size="compact"
					disabled={!changed || blank || rename.fetching}
					loading={rename.fetching}
				>
					{__('Save', 'alphone')}
				</Button>
			}
			aside={
				<>
					<NameForm
						id={nameForm}
						form={form}
						name={name}
						saving={rename.fetching}
						validity={refusedName.validity}
						onEdit={(edited) => setDraft(edited === contact.name ? null : edited)}
						onSubmit={() => void submitRename()}
					/>
					<SectionTitle id={identitiesHeading}>{__('Identities', 'alphone')}</SectionTitle>
					<IdentityList contact={contact} labelledBy={identitiesHeading} />
					<AddIdentityForm contact={contact} />
					<Text className="alphone-contacts__created">
						{sprintf(__('Created %(date)s', 'alphone'), { date: formatDate(contact.createdAt) })}
					</Text>
				</>
			}
			asideLabel={__('Contact details', 'alphone')}
		>
			<ContactTasks contactId={contact.id} tasks={tasks} />
			<ContactPanels contactId={contact.id} panels={contactPanels} />
		</PageScreen>
	)
}

/**
 * Returns the error an identity write should show, naming the owner of a claim.
 * @param error - The failure the mutation answered with.
 * @returns The mapped error, or undefined when the write succeeded.
 */
function identityError(error: GraphFailure | undefined): Error | undefined {
	const owner = graphExtensions(error).ownerName
	if (typeof owner === 'string') {
		return new ValidationError(sprintf(__('Already on contact %(name)s.', 'alphone'), { name: owner }))
	}
	return graphError(error)
}

/**
 * Returns the callback refreshing the contact after a write.
 * @returns The refresh callback.
 */
function useContactRefresh() {
	const graph = useGraph()
	return () => {
		graph.refetch([contactDetailOperation])
	}
}

/**
 * Returns the line one identity shows: its channel name, its identifier and any label.
 * @param identity - The identity to describe.
 * @returns The identity line.
 */
function identityText(identity: ContactDetail['identities'][number]): string {
	const text = `${channelName(identity.channel)}: ${identity.identifier}`
	return identity.displayName === '' ? text : `${text} (${identity.displayName})`
}

/**
 * Renders the contact's identities as a log list, each with its trash at the row end, or a placeholder when none exist.
 * @param props - The contact and the id of the heading that names the list.
 * @returns The identity list.
 */
function IdentityList({ contact, labelledBy }: { contact: ContactDetail; labelledBy: string }) {
	const settled = useContactRefresh()
	const toaster = useToaster()
	const [removing, setRemoving] = useState<readonly string[]>([])
	const [, runRemove] = useGraphMutation(deleteContactIdentityMutation)
	const removeIdentity = async (identityId: string) => {
		setRemoving((pending) => [...pending, identityId])
		const result = await runRemove({ contactId: contact.id, identityId })
		setRemoving((pending) => pending.filter((id) => id !== identityId))
		if (result.data) {
			toaster.show(__('Identity removed.', 'alphone'))
			settled()
		} else {
			toaster.show(writeFailure(__('Could not remove the identity.', 'alphone'), result.error))
		}
	}

	if (contact.identities.length === 0) {
		return <Text role="status">{__('No identities yet.', 'alphone')}</Text>
	}
	return (
		<LogList aria-labelledby={labelledBy}>
			{contact.identities.map((identity) => (
				<LogItem
					key={identity.id}
					aria-label={identityText(identity)}
					label={<Text>{identityText(identity)}</Text>}
					actions={
						<IconButton
							icon={trash}
							variant="minimal"
							tone="neutral"
							size="compact"
							label={sprintf(__('Remove %(identifier)s', 'alphone'), { identifier: identity.identifier })}
							loading={removing.includes(identity.id)}
							onClick={() => void removeIdentity(identity.id)}
						/>
					}
				/>
			))}
		</LogList>
	)
}

/**
 * Renders the form that attaches a new identity to the contact.
 * @returns The add identity form.
 */
function AddIdentityForm({ contact }: { contact: ContactDetail }) {
	const settled = useContactRefresh()
	const toaster = useToaster()
	const [channel, setChannel] = useState('email')
	const [identifier, setIdentifier] = useState('')
	const [label, setLabel] = useState('')
	const [add, runAdd] = useGraphMutation(addContactIdentityMutation)
	const form = useRef<HTMLFormElement>(null)
	const refusedValue = useRefusedField(form, identifier)
	const channels = channelItems()
	const submitIdentity = async () => {
		const result = await runAdd({
			contactId: contact.id,
			identity: { channel, identifier, displayName: label },
		})
		if (result.data) {
			setIdentifier('')
			setLabel('')
			toaster.show(__('Identity added.', 'alphone'))
			settled()
			return
		}
		const failure = identityError(result.error)
		const aboutChannel = channelReasons.includes(graphExtensions(result.error).reason)
		if (aboutChannel || !refusedValue.refuse(failure, identifier)) {
			toaster.show(writeFailure(__('Could not add the identity.', 'alphone'), failure))
		}
	}

	return (
		<form
			ref={form}
			className="godmin-form godmin-form--inline"
			onSubmit={(event) => {
				event.preventDefault()
				void submitIdentity()
			}}
		>
			<div className="godmin-form__row">
				<SelectControl
					label={__('Channel', 'alphone')}
					items={channels}
					value={channels.find((option) => option.value === channel)}
					onValueChange={(item) => {
						refusedValue.forget()
						setChannel(channelItemOf(item).value)
					}}
				/>
				<ValidatedInputControl
					label={__('Value', 'alphone')}
					value={identifier}
					customValidity={refusedValue.validity}
					onChange={(event) => setIdentifier(event.target.value)}
				/>
				<InputControl
					label={__('Label', 'alphone')}
					value={label}
					onChange={(event) => setLabel(event.target.value)}
				/>
				<Button
					type="submit"
					disabled={identifier.trim() === '' || add.fetching}
					loading={add.fetching}
				>
					{__('Add identity', 'alphone')}
				</Button>
			</div>
		</form>
	)
}

/**
 * Renders the name field in the form the page's Save button submits.
 * @param props - The form's id and element, the name shown, whether it is saving, its validity and the two callbacks.
 * @returns The name form.
 */
function NameForm({
	id,
	form,
	name,
	saving,
	validity,
	onEdit,
	onSubmit,
}: {
	id: string
	form: RefObject<HTMLFormElement | null>
	name: string
	saving: boolean
	validity: ComponentProps<typeof ValidatedInputControl>['customValidity']
	onEdit: (name: string) => void
	onSubmit: () => void
}) {
	return (
		<form
			ref={form}
			id={id}
			className="godmin-form"
			onSubmit={(event) => {
				event.preventDefault()
				onSubmit()
			}}
		>
			<ValidatedInputControl
				label={__('Name', 'alphone')}
				value={name}
				readOnly={saving}
				customValidity={validity}
				onChange={(event) => onEdit(event.target.value)}
			/>
		</form>
	)
}
