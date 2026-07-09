// SPDX-License-Identifier: AGPL-3.0-or-later

/// <reference types="vitest/config" />
import react from '@vitejs/plugin-react'
import dsTokenFallbacks from '@wordpress/theme/vite-plugins/vite-ds-token-fallbacks'
import { defineConfig } from 'vite'

export default defineConfig({
	plugins: [react(), dsTokenFallbacks()],
	resolve: {
		dedupe: [
			'react',
			'react-dom',
			'@tanstack/react-query',
			'@tanstack/react-router',
			'@wordpress/theme',
			'@wordpress/ui',
		],
	},
	server: {
		proxy: {
			'/api': 'http://localhost:8080',
		},
	},
	test: {
		environment: 'jsdom',
		setupFiles: ['./src/test/setup.ts'],
		include: [
			'src/**/*.test.{ts,tsx}',
			'../sdk/frontend/test/*.test.{ts,tsx}',
			'../plugins/*/frontend/test/*.test.{ts,tsx}',
		],
		coverage: {
			include: [
				'src/**',
				'../sdk/frontend/**/*.{ts,tsx}',
				'../plugins/*/frontend/**/*.{ts,tsx}',
			],
			exclude: ['src/main.tsx', '**/test/**', '**/node_modules/**'],
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
