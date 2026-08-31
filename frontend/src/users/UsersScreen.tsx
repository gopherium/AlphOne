// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	EmptyState,
	ErrorNotice,
	InputControl,
	LoadingRows,
	MANAGE_USERS,
	PageScreen,
	SelectControl,
	Stack,
	Text,
	VisuallyHidden,
	__,
	_x,
	can,
	sprintf,
	people,
	useSession,
} from '@alphone/frontend-sdk'
import { fetchUsers, setUserDisabled, usersQueryKey } from '@gopherium/react-auth/admin'
import type { Invitation } from '@gopherium/react-auth/admin'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import type { Account } from '../auth/graphTransport'
import { resendInvite, setUserRole } from '../auth/graphTransport'

/**
 * Returns the label a role reads as, falling back to what the server stored.
 * @param role - The role the account holds.
 * @returns The label.
 */
function roleLabel(role: string): string {
	if (role === '') {
		return __('No role', 'alphone')
	}
	return role.charAt(0).toUpperCase() + role.slice(1)
}

/**
 * Renders one account row with its status and tier, offering the controls only
 * to an admin looking at somebody else.
 * @param user - The account the row shows.
 * @param isSelf - Whether the row is the signed-in account, which gets no controls.
 * @param manages - Whether the caller may manage users at all.
 * @param grantable - The roles the caller may give another account.
 * @returns The table row element.
 */
function UserRow({
	user,
	isSelf,
	manages,
	grantable,
}: {
	user: Account
	isSelf: boolean
	manages: boolean
	grantable: string[]
}) {
	return (
		<tr>
			<td>{user.name}</td>
			<td>{user.email}</td>
			<td>
				<Badge intent={statusIntent(user)}>{statusLabel(user)}</Badge>
			</td>
			<td>
				{isSelf || !manages ? (
					roleLabel(user.role)
				) : (
					<UserRole user={user} grantable={grantable} />
				)}
			</td>
			<td className="godmin-table__actions">
				{isSelf || !manages ? null : <UserControls user={user} />}
			</td>
		</tr>
	)
}

/**
 * Renders the disable and tier controls one account offers an admin.
 * @param user - The account the controls act on.
 * @returns The control stack element.
 */
function UserControls({ user }: { user: Account }) {
	const queryClient = useQueryClient()
	const barred = user.disabled
	const toggle = useMutation({
		mutationFn: () => setUserDisabled(user.id, !barred),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
	})

	return (
		<Stack direction="column" gap="xs">
			<Button
				variant="outline"
				aria-label={sprintf(
					barred ? __('Enable %(name)s', 'alphone') : __('Disable %(name)s', 'alphone'),
					{ name: user.name },
				)}
				loading={toggle.isPending}
				onClick={() => toggle.mutate()}
			>
				{barred ? __('Enable', 'alphone') : __('Disable', 'alphone')}
			</Button>
			{user.confirmed ? null : <ResendControl user={user} />}
			{toggle.error ? <Text role="alert">{toggle.error.message}</Text> : null}
		</Stack>
	)
}

/**
 * Returns the label the status column reads for one account.
 * @param user - The account the row shows.
 * @returns The status label.
 */
function statusLabel(user: Account): string {
	if (user.disabled) {
		return _x('Disabled', 'account status', 'alphone')
	}
	if (!user.confirmed) {
		return _x('Invited', 'account status', 'alphone')
	}
	return _x('Active', 'account status', 'alphone')
}

/**
 * Returns the badge intent the status column carries for one account.
 * @param user - The account the row shows.
 * @returns The badge intent.
 */
function statusIntent(user: Account): 'draft' | 'stable' | 'informational' {
	if (user.disabled) {
		return 'draft'
	}
	if (!user.confirmed) {
		return 'informational'
	}
	return 'stable'
}

/**
 * Renders the control replacing a pending account's invitation.
 * @param user - The account awaiting activation.
 * @returns The resend control element.
 */
function ResendControl({ user }: { user: Account }) {
	const resend = useMutation<Invitation, Error>({
		mutationFn: () => resendInvite(user.email),
	})

	return (
		<Stack direction="column" gap="xs">
			<Button
				variant="outline"
				aria-label={sprintf(__('Resend invitation to %(name)s', 'alphone'), { name: user.name })}
				loading={resend.isPending}
				onClick={() => resend.mutate()}
			>
				{__('Resend invitation', 'alphone')}
			</Button>
			{resend.data?.delivered === true ? (
				<Text role="status">{__('Invitation sent.', 'alphone')}</Text>
			) : null}
			{resend.data?.delivered === false ? (
				<InputControl
					label={__('Activation link', 'alphone')}
					readOnly
					value={resend.data.activation_link}
				/>
			) : null}
			{resend.error ? <Text role="alert">{resend.error.message}</Text> : null}
		</Stack>
	)
}

/**
 * Renders the control writing the role an account holds.
 * @param user - The account the control acts on.
 * @param grantable - The roles the reader may give another account.
 * @returns The role control element.
 */
function UserRole({ user, grantable }: { user: Account; grantable: string[] }) {
	const queryClient = useQueryClient()
	const restand = useMutation({
		mutationFn: (role: string) => setUserRole(user.id, role),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: usersQueryKey }),
	})
	const offered = grantable.map((role) => ({ value: role, label: roleLabel(role) }))
	const held = offered.find((option) => option.value === user.role)

	return (
		<Stack direction="column" gap="xs">
			{held ? null : <Text>{roleLabel(user.role)}</Text>}
			<SelectControl
				label={sprintf(__('Role of %(name)s', 'alphone'), { name: user.name })}
				hideLabelFromVision
				disabled={restand.isPending}
				items={offered}
				value={held}
				onValueChange={(item) => item?.value != null && restand.mutate(item.value)}
			/>
			{restand.error ? <Text role="alert">{restand.error.message}</Text> : null}
		</Stack>
	)
}

/**
 * Renders the account list with its status and disable controls.
 * @returns The users screen.
 */
export function UsersScreen() {
	const session = useSession()
	const manages = can(session, MANAGE_USERS)
	const users = useQuery({
		queryKey: usersQueryKey,
		queryFn: ({ signal }) => fetchUsers(signal) as Promise<Account[]>,
	})

	return (
		<PageScreen
			title={__('Users', 'alphone')}
			actions={
				<Stack direction="row" gap="sm">
					<Button variant="outline" render={<Link to="/users/tokens" />}>
						{__('API tokens', 'alphone')}
					</Button>
					{manages ? (
						<Button variant="solid" render={<Link to="/users/new" />}>
							{__('New user', 'alphone')}
						</Button>
					) : null}
				</Stack>
			}
		>
			<UserRows
				users={users}
				currentUserId={session?.id}
				manages={manages}
				grantable={session?.grantable ?? []}
			/>
		</PageScreen>
	)
}

/**
 * Renders the loading, error, empty, and loaded states of the account list.
 * @returns The list body.
 */
function UserRows({
	users,
	currentUserId,
	manages,
	grantable,
}: {
	users: ReturnType<typeof useQuery<Account[], Error>>
	currentUserId: string | undefined
	manages: boolean
	grantable: string[]
}) {
	if (users.isPending) {
		return <LoadingRows label={__('Loading users…', 'alphone')} />
	}
	if (users.isError) {
		return <ErrorNotice>{__('Users could not be loaded.', 'alphone')}</ErrorNotice>
	}
	if (users.data.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>{__('No users yet.', 'alphone')}</EmptyState.Title>
				<EmptyState.Description>{__('Add one with New user.', 'alphone')}</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div
			className="godmin-table-scroll godmin-arrival"
			role="region"
			aria-label={__('Users', 'alphone')}
			tabIndex={0}
		>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">{__('Name', 'alphone')}</th>
						<th scope="col">{__('Email', 'alphone')}</th>
						<th scope="col">{__('Status', 'alphone')}</th>
						<th scope="col">{__('Role', 'alphone')}</th>
						<th scope="col" className="godmin-table__actions">
							<VisuallyHidden>{__('Actions', 'alphone')}</VisuallyHidden>
						</th>
					</tr>
				</thead>
				<tbody>
					{users.data.map((user) => (
						<UserRow
							key={user.id}
							user={user}
							isSelf={user.id === currentUserId}
							manages={manages}
							grantable={grantable}
						/>
					))}
				</tbody>
			</table>
		</div>
	)
}
