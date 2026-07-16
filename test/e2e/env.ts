// SPDX-License-Identifier: AGPL-3.0-or-later

import { fileURLToPath } from 'node:url'

export const repoRoot = fileURLToPath(new URL('../..', import.meta.url))

export const authFile = fileURLToPath(new URL('.auth/user.json', import.meta.url))

export const baseURL = process.env.ALPHONE_E2E_URL ?? 'http://localhost:8080'

export const credentials = {
	email: process.env.ALPHONE_E2E_EMAIL ?? 'e2e@example.com',
	password: process.env.ALPHONE_E2E_PASSWORD ?? 'correct horse battery',
	name: process.env.ALPHONE_E2E_NAME ?? 'Grace Hopper',
}
