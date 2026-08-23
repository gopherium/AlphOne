// SPDX-License-Identifier: AGPL-3.0-or-later

import {
	Button,
	EmptyState,
	ErrorNotice,
	LoadingRows,
	PageScreen,
	Stack,
	Text,
	VisuallyHidden,
	__,
	people,
	sprintf,
	useGraphMutation,
	useGraphQuery,
} from '@alphone/frontend-sdk'
import { Link } from '@tanstack/react-router'

import { formatCreated } from '../contacts/format'
import { formatExpiry, formatLastUsed } from './tokenFormat'
import { apiTokenRevokeMutation, apiTokensQuery } from './tokenOperations'

/** ApiTokenRow is one token as the listing shows it. */
interface ApiTokenRow {
	id: string
	name: string
	scopes: readonly string[]
	createdAt: string
	lastUsedAt?: string | null
	expiresAt?: string | null
}

/**
 * Renders one token row with its scopes, dates, and revoke control.
 * @returns The table row element.
 */
function TokenRow({ token, onRevoked }: { token: ApiTokenRow; onRevoked: () => void }) {
	const [revoke, runRevoke] = useGraphMutation(apiTokenRevokeMutation)
	const submit = async () => {
		const result = await runRevoke({ id: token.id })
		if (result.data) {
			onRevoked()
		}
	}

	return (
		<tr>
			<td>{token.name}</td>
			<td>{token.scopes.join(' ')}</td>
			<td>{formatCreated(new Date(token.createdAt))}</td>
			<td>{formatLastUsed(token.lastUsedAt)}</td>
			<td>{formatExpiry(token.expiresAt, new Date())}</td>
			<td className="godmin-table__actions">
				<Stack direction="column" gap="xs">
					<Button
						variant="outline"
						aria-label={sprintf(__('Revoke %(name)s', 'alphone'), { name: token.name })}
						loading={revoke.fetching}
						onClick={() => void submit()}
					>
						{__('Revoke', 'alphone')}
					</Button>
					{revoke.error ? <Text role="alert">{__('Revoke failed.', 'alphone')}</Text> : null}
				</Stack>
			</td>
		</tr>
	)
}

/**
 * Renders the caller's own API tokens with their scopes, dates, and revoke controls.
 * @returns The tokens screen.
 */
export function TokensScreen() {
	const [result, refetch] = useGraphQuery({
		query: apiTokensQuery,
		requestPolicy: 'cache-and-network',
	})

	return (
		<PageScreen
			title={__('API tokens', 'alphone')}
			actions={
				<Button variant="solid" render={<Link to="/users/tokens/new" />}>
					{__('New token', 'alphone')}
				</Button>
			}
		>
			<TokenRows
				tokens={result.data?.apiTokens}
				failed={result.error !== undefined}
				onRevoked={() => refetch({ requestPolicy: 'network-only' })}
			/>
		</PageScreen>
	)
}

/**
 * Renders the loading, error, empty, and loaded states of the token list.
 * @returns The list body.
 */
function TokenRows({
	tokens,
	failed,
	onRevoked,
}: {
	tokens: readonly ApiTokenRow[] | undefined
	failed: boolean
	onRevoked: () => void
}) {
	if (failed) {
		return <ErrorNotice>{__('Tokens could not be loaded.', 'alphone')}</ErrorNotice>
	}
	if (tokens === undefined) {
		return <LoadingRows label={__('Loading tokens…', 'alphone')} />
	}
	if (tokens.length === 0) {
		return (
			<EmptyState.Root className="godmin-empty">
				<EmptyState.Icon icon={people} />
				<EmptyState.Title>{__('No API tokens yet.', 'alphone')}</EmptyState.Title>
				<EmptyState.Description>{__('Add one with New token.', 'alphone')}</EmptyState.Description>
			</EmptyState.Root>
		)
	}
	return (
		<div
			className="godmin-table-scroll godmin-arrival"
			role="region"
			aria-label={__('API tokens', 'alphone')}
			tabIndex={0}
		>
			<table className="godmin-table">
				<thead>
					<tr>
						<th scope="col">{__('Name', 'alphone')}</th>
						<th scope="col">{__('Scopes', 'alphone')}</th>
						<th scope="col">{__('Created', 'alphone')}</th>
						<th scope="col">{__('Last used', 'alphone')}</th>
						<th scope="col">{__('Expires', 'alphone')}</th>
						<th scope="col" className="godmin-table__actions">
							<VisuallyHidden>{__('Actions', 'alphone')}</VisuallyHidden>
						</th>
					</tr>
				</thead>
				<tbody>
					{tokens.map((token) => (
						<TokenRow key={token.id} token={token} onRevoked={onRevoked} />
					))}
				</tbody>
			</table>
		</div>
	)
}
