// SPDX-License-Identifier: Elastic-2.0

import { execFileSync } from 'node:child_process'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { networkInterfaces, tmpdir } from 'node:os'
import { join } from 'node:path'

import { cases } from './cases.ts'
import type { Case, Context, Item } from './cases.ts'

/** NodeRun is what one node left behind in an execution record. */
interface NodeRun {
	data?: { main?: { json: Item }[][] }
	error?: { message?: string }
}

/** ExecutionRecord is what the n8n CLI prints for one execution. */
interface ExecutionRecord {
	data?: {
		resultData?: {
			runData?: Record<string, NodeRun[]>
			error?: { message?: string }
		}
	}
}

/** Outcome is what one executed case answered with. */
interface Outcome {
	items: Item[]
	failure: string | null
}

/** service is the compose service running n8n. */
const service = process.env.RUNBOOK_N8N_SERVICE ?? 'n8n'

/** brokerPort keeps the CLI task broker off the port the running n8n holds. */
const brokerPort = process.env.RUNBOOK_BROKER_PORT ?? '5699'

/** hostURL is where this script reaches AlphOne. */
const hostURL = process.env.ALPHONE_URL ?? 'http://localhost:8080'

/** token is the API token every workflow authenticates with. */
const token = process.env.ALPHONE_TOKEN ?? ''

/** credentialID names the credential every generated workflow carries. */
const credentialID = 'alphoneRunbook'

/** answered matches what AlphOne itself replies with, rather than any reachable host. */
const answered = /\b401\b|"version"/

/** work is the scratch directory generated files are staged in. */
const work = mkdtempSync(join(tmpdir(), 'alphone-runbook-'))

/**
 * Runs a docker compose command.
 * @param args - The arguments following the compose command.
 * @returns The standard output the command wrote.
 */
function compose(args: string[]): string {
	return execFileSync('docker', ['compose', ...args], { encoding: 'utf8', maxBuffer: 64 << 20 })
}

/**
 * Runs a command inside the n8n container, its exit code aside.
 * @param command - The shell command to run in the container.
 * @returns The standard output the command wrote.
 */
function inN8n(command: string): string {
	const args = ['exec', '-T', '-e', `N8N_RUNNERS_BROKER_PORT=${brokerPort}`, service, 'sh', '-c', command]
	try {
		return compose(args)
	} catch (error) {
		return String((error as { stdout?: string }).stdout ?? '')
	}
}

/**
 * Stages a file and copies it into the n8n container.
 * @param name - The file name to stage it under.
 * @param body - The contents to write.
 * @returns The path the file landed on inside the container.
 */
function copyIn(name: string, body: string): string {
	const path = join(work, name)
	writeFileSync(path, body)
	compose(['cp', path, `${service}:/tmp/${name}`])
	return `/tmp/${name}`
}

/**
 * Reports whether the n8n container gets AlphOne's own answer from an address.
 * @param host - The address to try.
 * @param port - The port AlphOne listens on.
 * @returns Whether AlphOne answered rather than some other host.
 */
function reaches(host: string, port: string): boolean {
	return answered.test(
		inN8n(`wget -q -O- --timeout=2 http://${host}:${port}/api/version 2>&1 || true`),
	)
}

/**
 * Returns the default route the n8n container leaves through.
 * @returns The gateway address, empty when the container reports none.
 */
function gatewayHost(): string {
	return /default via (\S+)/.exec(inN8n('ip route'))?.[1] ?? ''
}

/**
 * Returns the outward facing addresses of this machine.
 * @returns Every non loopback IPv4 address.
 */
function localHosts(): string[] {
	return Object.values(networkInterfaces())
		.flat()
		.flatMap((entry) => (entry?.family === 'IPv4' && !entry.internal ? [entry.address] : []))
}

/**
 * Returns the addresses a container may reach this machine on.
 * @returns The addresses to try, most conventional first.
 */
function candidateHosts(): string[] {
	return ['host.docker.internal', gatewayHost(), ...localHosts()].filter(Boolean)
}

/**
 * Returns the AlphOne address the n8n container can reach.
 * @returns The base URL every generated credential carries.
 */
function reachableURL(): string {
	if (process.env.ALPHONE_CONTAINER_URL) {
		return process.env.ALPHONE_CONTAINER_URL
	}
	const port = new URL(hostURL).port || '8080'
	const host = candidateHosts().find((candidate) => reaches(candidate, port))
	if (!host) {
		throw new Error('no address reached AlphOne from the n8n container, set ALPHONE_CONTAINER_URL')
	}
	return `http://${host}:${port}`
}

/**
 * Returns the error a refused AlphOne call earns.
 * @param init - The request that was made.
 * @param path - The path it was made against.
 * @param status - The status AlphOne answered with.
 * @returns The error to raise.
 */
function refusedCall(init: RequestInit, path: string, status: number): Error {
	return new Error(`${init.method ?? 'GET'} ${path} answered ${status}`)
}

/**
 * Calls AlphOne as the runbook token.
 * @param path - The path to call, base URL aside.
 * @param init - The request to make, its authorization aside.
 * @returns The parsed answer, or null for an empty one.
 */
async function callAlphOne(path: string, init: RequestInit = {}): Promise<unknown> {
	const response = await fetch(`${hostURL}${path}`, {
		...init,
		headers: { Authorization: `Bearer ${token}`, ...(init.headers ?? {}) },
	})
	if (!response.ok) {
		throw refusedCall(init, path, response.status)
	}
	return response.status === 204 ? null : response.json()
}

/**
 * Stores the contact the search case has to single out.
 */
async function seedContacts(): Promise<void> {
	await callAlphOne('/api/contacts', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ name: 'Ada Lovelace' }),
	})
}

/**
 * Uploads, maps and commits one import so the import cases have data.
 * @param runID - What keeps this run's imported identity unique.
 */
async function seedImport(runID: number): Promise<void> {
	const csv = `Name,Email\nGrace Hopper,grace.${runID}@example.com\n`
	const form = new FormData()
	form.append('file', new Blob([csv], { type: 'text/csv' }), 'contacts.csv')
	const uploaded = (await callAlphOne('/api/plugins/importer/imports', {
		method: 'POST',
		body: form,
	})) as { id: string }
	await callAlphOne(`/api/plugins/importer/imports/${uploaded.id}/mapping`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			assignments: [
				{ column: 0, field: 'name' },
				{ column: 1, field: 'email' },
			],
		}),
	})
	await callAlphOne(`/api/plugins/importer/imports/${uploaded.id}/commit`, { method: 'POST' })
}

/**
 * Stores the AlphOne credential every generated workflow carries.
 * @param containerURL - Where the container reaches AlphOne.
 */
function importCredential(containerURL: string): void {
	const body = JSON.stringify([
		{
			id: credentialID,
			name: 'AlphOne runbook',
			type: 'alphOneApi',
			data: { baseUrl: containerURL, apiToken: token },
		},
	])
	const output = inN8n(`n8n import:credentials -i ${copyIn('runbook-credentials.json', body)}`)
	if (!output.includes('Successfully imported')) {
		throw new Error(`importing the credential failed: ${output.trim()}`)
	}
}

/**
 * Returns the one node workflow driving a case.
 * @param id - The workflow id to import it under.
 * @param parameters - The node parameters the case runs with.
 * @returns The workflow list the n8n importer takes.
 */
function workflowFor(id: string, parameters: Record<string, unknown>): unknown {
	return [
		{
			id,
			name: id,
			active: false,
			settings: {},
			nodes: [
				{
					parameters: {},
					id: '11111111-1111-4111-8111-111111111111',
					name: 'Start',
					type: 'n8n-nodes-base.manualTrigger',
					typeVersion: 1,
					position: [0, 0],
				},
				{
					parameters,
					id: '22222222-2222-4222-8222-222222222222',
					name: 'AlphOne',
					type: 'n8n-nodes-alphone.alphOne',
					typeVersion: 1,
					position: [220, 0],
					credentials: { alphOneApi: { id: credentialID, name: 'AlphOne runbook' } },
				},
			],
			connections: { Start: { main: [[{ node: 'AlphOne', type: 'main', index: 0 }]] } },
		},
	]
}

/**
 * Returns the execution record the CLI printed, trailing complaints aside.
 * @param raw - Everything the CLI wrote.
 * @returns The parsed record.
 */
function parseExecution(raw: string): ExecutionRecord {
	const start = raw.search(/^\{$/m)
	const end = raw.lastIndexOf('\n}')
	if (start === -1 || end === -1) {
		throw new Error(`the CLI printed no execution record: ${raw.trim().slice(0, 200)}`)
	}
	return JSON.parse(raw.slice(start, end + 2)) as ExecutionRecord
}

/**
 * Returns what the AlphOne node left behind in an execution record.
 * @param record - The parsed execution.
 * @returns The node's run, or undefined when it never ran.
 */
function nodeRunOf(record: ExecutionRecord): NodeRun | undefined {
	return record.data?.resultData?.runData?.AlphOne?.[0]
}

/**
 * Returns the items one node run answered with.
 * @param run - The node's run.
 * @returns The answered records.
 */
function itemsOf(run: NodeRun | undefined): Item[] {
	return (run?.data?.main?.[0] ?? []).map((item) => item.json)
}

/**
 * Returns the failure an execution recorded as a whole.
 * @param record - The parsed execution.
 * @returns The message, or undefined when the execution finished.
 */
function executionError(record: ExecutionRecord): string | undefined {
	return record.data?.resultData?.error?.message
}

/**
 * Returns the failure one node run recorded.
 * @param run - The node's run.
 * @returns The message, or undefined when the node finished.
 */
function nodeError(run: NodeRun | undefined): string | undefined {
	return run?.error?.message
}

/**
 * Imports and executes one case.
 * @param id - The workflow id to run it under.
 * @param parameters - The node parameters the case runs with.
 * @returns The items it answered with beside any failure.
 */
function runCase(id: string, parameters: Record<string, unknown>): Outcome {
	const file = copyIn(`${id}.json`, JSON.stringify(workflowFor(id, parameters)))
	const imported = inN8n(`n8n import:workflow -i ${file}`)
	if (!imported.includes('Successfully imported')) {
		throw new Error(`importing the ${id} workflow failed: ${imported.trim()}`)
	}
	const record = parseExecution(inN8n(`n8n execute --id=${id} --rawOutput`))
	const run = nodeRunOf(record)
	return { items: itemsOf(run), failure: executionError(record) ?? nodeError(run) ?? null }
}

/**
 * Returns the complaint one executed case earned.
 * @param one - The case that ran.
 * @param result - What it answered with.
 * @param ctx - The ids the cases thread between them.
 * @returns The complaint, or null when the case passed.
 */
function verdictOf(one: Case, result: Outcome, ctx: Context): string | null {
	if (one.expectFailure) {
		return result.failure ? null : 'the node succeeded, want it to fail'
	}
	if (result.failure) {
		return `the node failed: ${result.failure}`
	}
	return one.check ? one.check(result.items, ctx) : null
}

/**
 * Runs one case as many times as it asks for.
 * @param one - The case to drive.
 * @param ctx - The ids the cases thread between them.
 * @returns The complaint the case earned, or null when it passed.
 */
function driveCase(one: Case, ctx: Context): string | null {
	const id = one.name.replaceAll(/[^a-zA-Z0-9]/g, '')
	let verdict: string | null = null
	for (let attempt = 0; attempt < (one.repeat ?? 1); attempt += 1) {
		verdict = verdictOf(one, runCase(id, one.params(ctx)), ctx)
		if (verdict) {
			return verdict
		}
	}
	return verdict
}

/**
 * Returns the report line one driven case earns.
 * @param one - The case that ran.
 * @param verdict - The complaint it earned, or null when it passed.
 * @returns The line to print.
 */
function reportLine(one: Case, verdict: string | null): string {
	return `${verdict ? 'FAIL' : 'ok  '} ${one.name}${verdict ? `  ${verdict}` : ''}`
}

/**
 * Drives every case and reports how many failed.
 */
async function main(): Promise<void> {
	if (!token) {
		throw new Error('set ALPHONE_TOKEN to a token minted with: alphone token create -name runbook')
	}
	const containerURL = reachableURL()
	console.log(`n8n reaches AlphOne at ${containerURL}`)
	await seedContacts()
	await seedImport(Date.now())
	importCredential(containerURL)

	const ctx: Context = { today: new Date().toISOString().slice(0, 10) }
	let failed = 0
	for (const one of cases) {
		const verdict = driveCase(one, ctx)
		console.log(reportLine(one, verdict))
		failed += verdict ? 1 : 0
	}
	console.log(`\n${cases.length} cases, ${failed} failed`)
	process.exit(failed === 0 ? 0 : 1)
}

await main()
