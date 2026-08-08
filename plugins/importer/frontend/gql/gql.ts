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
    "\n\tquery Imports {\n\t\timports {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\trowCount\n\t\t\timportedCount\n\t\t\tskippedCount\n\t\t\tfailedCount\n\t\t\tcreatedAt\n\t\t}\n\t}\n": typeof types.ImportsDocument,
    "\n\tquery ImportDetail($id: UUID!) {\n\t\timportJob(id: $id) {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\tcolumns\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t\trows {\n\t\t\t\tid\n\t\t\t\tposition\n\t\t\t\tcells\n\t\t\t\toutcome\n\t\t\t\treason\n\t\t\t}\n\t\t}\n\t\timportFields {\n\t\t\tname\n\t\t\tlabel\n\t\t\trequired\n\t\t}\n\t}\n": typeof types.ImportDetailDocument,
    "\n\tmutation ImportUpload($file: Upload!) {\n\t\timportUpload(file: $file) {\n\t\t\tid\n\t\t\tfilename\n\t\t}\n\t}\n": typeof types.ImportUploadDocument,
    "\n\tmutation ImportSetMapping($id: UUID!, $assignments: [ImportAssignmentInput!]!) {\n\t\timportSetMapping(id: $id, assignments: $assignments) {\n\t\t\tid\n\t\t\tstate\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t}\n\t}\n": typeof types.ImportSetMappingDocument,
    "\n\tmutation ImportCommit($id: UUID!) {\n\t\timportCommit(id: $id) {\n\t\t\tid\n\t\t\timported\n\t\t\tskipped\n\t\t\tfailed\n\t\t}\n\t}\n": typeof types.ImportCommitDocument,
};
const documents: Documents = {
    "\n\tquery Imports {\n\t\timports {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\trowCount\n\t\t\timportedCount\n\t\t\tskippedCount\n\t\t\tfailedCount\n\t\t\tcreatedAt\n\t\t}\n\t}\n": types.ImportsDocument,
    "\n\tquery ImportDetail($id: UUID!) {\n\t\timportJob(id: $id) {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\tcolumns\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t\trows {\n\t\t\t\tid\n\t\t\t\tposition\n\t\t\t\tcells\n\t\t\t\toutcome\n\t\t\t\treason\n\t\t\t}\n\t\t}\n\t\timportFields {\n\t\t\tname\n\t\t\tlabel\n\t\t\trequired\n\t\t}\n\t}\n": types.ImportDetailDocument,
    "\n\tmutation ImportUpload($file: Upload!) {\n\t\timportUpload(file: $file) {\n\t\t\tid\n\t\t\tfilename\n\t\t}\n\t}\n": types.ImportUploadDocument,
    "\n\tmutation ImportSetMapping($id: UUID!, $assignments: [ImportAssignmentInput!]!) {\n\t\timportSetMapping(id: $id, assignments: $assignments) {\n\t\t\tid\n\t\t\tstate\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t}\n\t}\n": types.ImportSetMappingDocument,
    "\n\tmutation ImportCommit($id: UUID!) {\n\t\timportCommit(id: $id) {\n\t\t\tid\n\t\t\timported\n\t\t\tskipped\n\t\t\tfailed\n\t\t}\n\t}\n": types.ImportCommitDocument,
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
export function graphql(source: "\n\tquery Imports {\n\t\timports {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\trowCount\n\t\t\timportedCount\n\t\t\tskippedCount\n\t\t\tfailedCount\n\t\t\tcreatedAt\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Imports {\n\t\timports {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\trowCount\n\t\t\timportedCount\n\t\t\tskippedCount\n\t\t\tfailedCount\n\t\t\tcreatedAt\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery ImportDetail($id: UUID!) {\n\t\timportJob(id: $id) {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\tcolumns\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t\trows {\n\t\t\t\tid\n\t\t\t\tposition\n\t\t\t\tcells\n\t\t\t\toutcome\n\t\t\t\treason\n\t\t\t}\n\t\t}\n\t\timportFields {\n\t\t\tname\n\t\t\tlabel\n\t\t\trequired\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery ImportDetail($id: UUID!) {\n\t\timportJob(id: $id) {\n\t\t\tid\n\t\t\tfilename\n\t\t\tstate\n\t\t\tcolumns\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t\trows {\n\t\t\t\tid\n\t\t\t\tposition\n\t\t\t\tcells\n\t\t\t\toutcome\n\t\t\t\treason\n\t\t\t}\n\t\t}\n\t\timportFields {\n\t\t\tname\n\t\t\tlabel\n\t\t\trequired\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation ImportUpload($file: Upload!) {\n\t\timportUpload(file: $file) {\n\t\t\tid\n\t\t\tfilename\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation ImportUpload($file: Upload!) {\n\t\timportUpload(file: $file) {\n\t\t\tid\n\t\t\tfilename\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation ImportSetMapping($id: UUID!, $assignments: [ImportAssignmentInput!]!) {\n\t\timportSetMapping(id: $id, assignments: $assignments) {\n\t\t\tid\n\t\t\tstate\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation ImportSetMapping($id: UUID!, $assignments: [ImportAssignmentInput!]!) {\n\t\timportSetMapping(id: $id, assignments: $assignments) {\n\t\t\tid\n\t\t\tstate\n\t\t\tmapping {\n\t\t\t\tcolumn\n\t\t\t\tfield\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation ImportCommit($id: UUID!) {\n\t\timportCommit(id: $id) {\n\t\t\tid\n\t\t\timported\n\t\t\tskipped\n\t\t\tfailed\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation ImportCommit($id: UUID!) {\n\t\timportCommit(id: $id) {\n\t\t\tid\n\t\t\timported\n\t\t\tskipped\n\t\t\tfailed\n\t\t}\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;