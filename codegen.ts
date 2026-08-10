// SPDX-License-Identifier: AGPL-3.0-or-later

import type { CodegenConfig } from '@graphql-codegen/cli'

const scalars = {
	UUID: 'string',
	DateTime: 'string',
	Date: 'string',
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

const config: CodegenConfig = {
	schema: ['graph/schema/*.graphqls', 'plugins/*/graph/*.graphqls'],
	generates: {
		...clientPreset('frontend/src'),
		...clientPreset('plugins/whatsapp/frontend'),
		...clientPreset('plugins/importer/frontend'),
	},
	ignoreNoDocuments: true,
}

export default config
