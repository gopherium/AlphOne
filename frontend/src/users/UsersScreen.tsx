// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Badge,
	Button,
	EmptyState,
	ErrorNotice,
	LoadingRows,
	MANAGE_USERS,
	PageScreen,
	SelectControl,
	Stack,
	Text,
	VisuallyHidden,
	can,
	people,
	useSession,
} from '@alphone/frontend-sdk'
import { fetchUsers, setUserDisabled, usersQueryKey } from '@gopherium/react-auth/admin'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'

import type { Account } from '../auth/graphTransport'
import { setUserRole } from '../auth/graphTransport'

/**
 * Returns the label a role reads as, falling back to what the server stored.
 * @param role - The role the account holds.
 * @returns The label.
 */
function roleLabel(role: string): string {
	if (role === '') {
		return 'No role'
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
				<Badge intent={user.disabled ? 'draft' : 'stable'}>
					{user.disabled ? 'Disabled' : 'Active'}
				</Badge>
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
				aria-label={`${barred ? 'Enable' : 'Disable'} ${user.name}`}
				loading={toggle.isPending}
				onClick={() => toggle.mutate()}
			>
				{barred ? 'Enable' : 'Disable'}
			</Button>
			{toggle.error ? <Text role="alert">{toggle.error.message}</Text> : null}
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
				label={`Role of ${user.name}`}
				hideLabelFromVision
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
			title="Users"
			actions={
				<Stack direction="row" gap="sm">
					<Button variant="outline" render={<Link to="/users/tokens" />}>
						API tokens
					</Button>
					{manages ? (
						<Button variant="solid" render={<Link to="/users/new" />}>
							New user
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
		return <LoadingRows label="Loading users…" />
	}
	if (users.isError) {
		return <ErrorNotice>Users could not be loaded.</ErrorNotice>
	}
	if (users.data.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>No users yet.</EmptyState.Title>
				<EmptyState.Description>Add one with New user.</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div
			className="godmin-table-scroll godmin-arrival"
			role="region"
			aria-label="Users"
			tabIndex={0}
		>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">Name</th>
						<th scope="col">Email</th>
						<th scope="col">Status</th>
						<th scope="col">Role</th>
						<th scope="col" className="godmin-table__actions">
							<VisuallyHidden>Actions</VisuallyHidden>
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
