// SPDX-License-Identifier: AGPL-3.0-or-later

import { globSync, readFileSync } from 'node:fs'
import { dirname } from 'node:path'

import type { CodegenConfig } from '@graphql-codegen/cli'

const scalars = {
	UUID: 'string',
	DateTime: 'string',
	Date: 'string',
	LocalTime: 'string',
	Upload: 'File',
}

/**
 * Builds the client preset output for one package's documents.
 * @param root - The package directory holding the documents.
 * @returns The generates entry writing that package's gql module.
 */
function clientPreset(root: string) {
	return {
		[`${root}/gql/`]: {
			preset: 'client' as const,
			presetConfig: { fragmentMasking: false },
			documents: [
			`${root}/**/*.{ts,tsx}`,
			`!${root}/gql/**`,
			`!${root}/node_modules/**`,
		],
			config: { useTypeImports: true, scalars },
		},
	}
}

/**
 * Reports whether the plugin manifest at path names a frontend.
 * @param path - The plugin.json to read.
 * @returns True when the manifest sets frontend.
 */
function namesFrontend(path: string): boolean {
	const manifest = JSON.parse(readFileSync(path, 'utf8')) as { frontend?: string }
	return Boolean(manifest.frontend)
}

/**
 * Returns the frontend folder of every plugin manifest that names a frontend.
 * @returns The folders under the plugins and enterprise roots, sorted.
 */
function pluginFrontends(): string[] {
	return globSync(['plugins/*/plugin.json', 'enterprise/*/plugin.json'])
		.filter(namesFrontend)
		.map((manifest) => `${dirname(manifest)}/frontend`)
		.sort()
}

const config: CodegenConfig = {
	schema: ['graph/schema/*.graphqls', 'plugins/*/graph/*.graphqls', 'enterprise/*/graph/*.graphqls'],
	generates: Object.assign(clientPreset('frontend/src'), ...pluginFrontends().map(clientPreset)),
	ignoreNoDocuments: true,
}

export default config
