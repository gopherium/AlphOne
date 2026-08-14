// SPDX-License-Identifier: AGPL-3.0-or-later

/// <reference types="vitest/config" />
import { godminDedupe, godminSingleCopy, godminStylesheetFirst } from '@gopherium/godmin/vite'
import react from '@vitejs/plugin-react'
import dsTokenFallbacks from '@wordpress/theme/vite-plugins/vite-ds-token-fallbacks'
import { defineConfig } from 'vite'

export default defineConfig({
	plugins: [react(), dsTokenFallbacks(), godminSingleCopy(), godminStylesheetFirst()],
	resolve: {
		dedupe: [
			...godminDedupe,
			'@tanstack/react-query',
			'@tanstack/react-router',
		],
	},
	server: {
		proxy: {
			'/api': process.env.ALPHONE_API || 'http://localhost:8080',
		},
	},
	test: {
		environment: 'jsdom',
		env: { TZ: 'UTC' },
		setupFiles: ['./src/test/setup.ts'],
		include: [
			'src/**/*.test.{ts,tsx}',
			'../sdk/*/test/*.test.{ts,tsx}',
			'../plugins/*/frontend/test/*.test.{ts,tsx}',
			'../enterprise/*/frontend/test/*.test.{ts,tsx}',
		],
		coverage: {
			include: [
				'src/**',
				'../sdk/*/**/*.{ts,tsx}',
				'../plugins/*/frontend/**/*.{ts,tsx}',
				'../enterprise/*/frontend/**/*.{ts,tsx}',
			],
			exclude: ['src/main.tsx', '**/gql/**', '**/test/**', '**/node_modules/**', '**/*.d.ts'],
			allowExternal: true,
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
