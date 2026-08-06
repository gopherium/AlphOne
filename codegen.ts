import type { CodegenConfig } from '@graphql-codegen/cli'

const config: CodegenConfig = {
	schema: ['graph/schema/*.graphqls', 'plugins/*/graph/*.graphqls'],
	documents: ['frontend/src/**/*.{ts,tsx}', '!frontend/src/gql/**'],
	generates: {
		'frontend/src/gql/': {
			preset: 'client',
			config: {
				useTypeImports: true,
				scalars: {
					UUID: 'string',
					DateTime: 'string',
					Date: 'string',
				},
			},
		},
	},
	ignoreNoDocuments: true,
}

export default config
