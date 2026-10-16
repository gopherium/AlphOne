// SPDX-License-Identifier: AGPL-3.0-or-later

/// <reference types="vitest/config" />
import { fileURLToPath } from 'node:url'

import { godminDedupe, godminSingleCopy, godminStylesheetFirst } from '@gopherium/godmin/vite'
import react from '@vitejs/plugin-react'
import dsTokenFallbacks from '@wordpress/theme/vite-plugins/vite-ds-token-fallbacks'
import { defineConfig, normalizePath } from 'vite'

/** Resolves a repository path to an absolute path. */
function repoGlob(pattern: string): string {
	return normalizePath(fileURLToPath(new URL(`../${pattern}`, import.meta.url)))
}

export default defineConfig({
	plugins: [react(), dsTokenFallbacks(), godminSingleCopy(), godminStylesheetFirst()],
	resolve: {
		dedupe: [
			...godminDedupe,
			'@gopherium/gottext',
			'@tanstack/react-query',
			'@tanstack/react-router',
			'@wordpress/i18n',
		],
	},
	server: {
		proxy: {
			'/api': process.env.ALPHONE_API || 'http://localhost:8080',
		},
	},
	test: {
		root: repoGlob(''),
		environment: 'jsdom',
		env: { TZ: 'UTC' },
		server: {
			deps: {
				inline: [
					'@wordpress/i18n',
					'@gopherium/gottext',
					'@gopherium/godmin',
					'@gopherium/react-auth',
				],
			},
		},
		setupFiles: [repoGlob('frontend/src/test/setup.ts')],
		include: [
			'frontend/src/**/*.test.{ts,tsx}',
			'sdk/*/test/*.test.{ts,tsx}',
			'plugins/*/frontend/test/*.test.{ts,tsx}',
			'enterprise/*/frontend/test/*.test.{ts,tsx}',
		],
		coverage: {
			include: [
				'frontend/src/**',
				'sdk/*/**/*.{ts,tsx}',
				'plugins/*/frontend/**/*.{ts,tsx}',
				'enterprise/*/frontend/**/*.{ts,tsx}',
			],
			exclude: ['frontend/src/main.tsx', '**/gql/**', '**/test/**', '**/node_modules/**', '**/*.d.ts'],
			allowExternal: true,
			reportsDirectory: repoGlob('frontend/coverage'),
			reporter: ['text', 'lcov'],
			thresholds: {
				statements: 100,
				branches: 100,
				functions: 100,
				lines: 100,
			},
		},
	},
})
