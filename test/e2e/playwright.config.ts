// SPDX-License-Identifier: AGPL-3.0-or-later

import { defineConfig, devices } from '@playwright/test'

import { authFile, baseURL, repoRoot } from './env'

export default defineConfig({
	testDir: './tests',
	fullyParallel: false,
	forbidOnly: !!process.env.CI,
	retries: process.env.CI ? 2 : 0,
	workers: 1,
	reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : 'list',
	use: {
		baseURL,
		trace: 'on-first-retry',
		screenshot: 'only-on-failure',
	},
	projects: [
		{ name: 'setup', testMatch: /.*\.setup\.ts/ },
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'], storageState: authFile },
			dependencies: ['setup'],
		},
	],
	webServer: {
		command: 'make e2e-serve',
		cwd: repoRoot,
		url: baseURL,
		reuseExistingServer: !process.env.CI,
		timeout: 180_000,
	},
})
