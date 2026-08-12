/* eslint-disable */
import * as types from './graphql';
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';

/**
 * Map of all GraphQL operations in the project.
 *
 * This map has several performance disadvantages:
 * 1. It is not tree-shakeable, so it will include all operations in the project.
 * 2. It is not minifiable, so the string of a GraphQL query will be multiple times inside the bundle.
 * 3. It does not support dead code elimination, so it will add unused operations.
 *
 * Therefore it is highly recommended to use the babel or swc plugin for production.
 * Learn more about it here: https://the-guild.dev/graphql/codegen/plugins/presets/preset-client#reducing-bundle-size
 */
type Documents = {
    "\n\tquery Fields {\n\t\tfields {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n": typeof types.FieldsDocument,
    "\n\tmutation DefineField($name: String!, $label: String!, $kind: FieldKind!) {\n\t\tdefineField(name: $name, label: $label, kind: $kind) {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n": typeof types.DefineFieldDocument,
    "\n\tmutation ArchiveField($id: UUID!) {\n\t\tarchiveField(id: $id)\n\t}\n": typeof types.ArchiveFieldDocument,
    "\n\tmutation WriteContactFields($contactId: UUID!, $values: JSON!) {\n\t\twriteContactFields(contactId: $contactId, values: $values)\n\t}\n": typeof types.WriteContactFieldsDocument,
};
const documents: Documents = {
    "\n\tquery Fields {\n\t\tfields {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n": types.FieldsDocument,
    "\n\tmutation DefineField($name: String!, $label: String!, $kind: FieldKind!) {\n\t\tdefineField(name: $name, label: $label, kind: $kind) {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n": types.DefineFieldDocument,
    "\n\tmutation ArchiveField($id: UUID!) {\n\t\tarchiveField(id: $id)\n\t}\n": types.ArchiveFieldDocument,
    "\n\tmutation WriteContactFields($contactId: UUID!, $values: JSON!) {\n\t\twriteContactFields(contactId: $contactId, values: $values)\n\t}\n": types.WriteContactFieldsDocument,
};

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 *
 *
 * @example
 * ```ts
 * const query = graphql(`query GetUser($id: ID!) { user(id: $id) { name } }`);
 * ```
 *
 * The query argument is unknown!
 * Please regenerate the types.
 */
export function graphql(source: string): unknown;

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery Fields {\n\t\tfields {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Fields {\n\t\tfields {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation DefineField($name: String!, $label: String!, $kind: FieldKind!) {\n\t\tdefineField(name: $name, label: $label, kind: $kind) {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation DefineField($name: String!, $label: String!, $kind: FieldKind!) {\n\t\tdefineField(name: $name, label: $label, kind: $kind) {\n\t\t\tid\n\t\t\tname\n\t\t\tlabel\n\t\t\tkind\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation ArchiveField($id: UUID!) {\n\t\tarchiveField(id: $id)\n\t}\n"): (typeof documents)["\n\tmutation ArchiveField($id: UUID!) {\n\t\tarchiveField(id: $id)\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation WriteContactFields($contactId: UUID!, $values: JSON!) {\n\t\twriteContactFields(contactId: $contactId, values: $values)\n\t}\n"): (typeof documents)["\n\tmutation WriteContactFields($contactId: UUID!, $values: JSON!) {\n\t\twriteContactFields(contactId: $contactId, values: $values)\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;