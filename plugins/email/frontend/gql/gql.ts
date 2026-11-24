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
    "\n\tquery EmailMailbox {\n\t\temailMailbox {\n\t\t\tid\n\t\t\taddress\n\t\t\taliases\n\t\t}\n\t}\n": typeof types.EmailMailboxDocument,
    "\n\tquery EmailInbox($limit: Int, $offset: Int) {\n\t\temailThreads(limit: $limit, offset: $offset) {\n\t\t\ttotal\n\t\t\tlimit\n\t\t\titems {\n\t\t\t\tid\n\t\t\t\tsubject\n\t\t\t\tlastMessageAt\n\t\t\t\tmessageCount\n\t\t\t\thasAttachments\n\t\t\t\tlatest {\n\t\t\t\t\tid\n\t\t\t\t\tfrom {\n\t\t\t\t\t\tname\n\t\t\t\t\t\temail\n\t\t\t\t\t}\n\t\t\t\t\tsnippet\n\t\t\t\t\tdirection\n\t\t\t\t\tencrypted\n\t\t\t\t\tsigned\n\t\t\t\t\tbodyState\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n": typeof types.EmailInboxDocument,
};
const documents: Documents = {
    "\n\tquery EmailMailbox {\n\t\temailMailbox {\n\t\t\tid\n\t\t\taddress\n\t\t\taliases\n\t\t}\n\t}\n": types.EmailMailboxDocument,
    "\n\tquery EmailInbox($limit: Int, $offset: Int) {\n\t\temailThreads(limit: $limit, offset: $offset) {\n\t\t\ttotal\n\t\t\tlimit\n\t\t\titems {\n\t\t\t\tid\n\t\t\t\tsubject\n\t\t\t\tlastMessageAt\n\t\t\t\tmessageCount\n\t\t\t\thasAttachments\n\t\t\t\tlatest {\n\t\t\t\t\tid\n\t\t\t\t\tfrom {\n\t\t\t\t\t\tname\n\t\t\t\t\t\temail\n\t\t\t\t\t}\n\t\t\t\t\tsnippet\n\t\t\t\t\tdirection\n\t\t\t\t\tencrypted\n\t\t\t\t\tsigned\n\t\t\t\t\tbodyState\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n": types.EmailInboxDocument,
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
export function graphql(source: "\n\tquery EmailMailbox {\n\t\temailMailbox {\n\t\t\tid\n\t\t\taddress\n\t\t\taliases\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery EmailMailbox {\n\t\temailMailbox {\n\t\t\tid\n\t\t\taddress\n\t\t\taliases\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery EmailInbox($limit: Int, $offset: Int) {\n\t\temailThreads(limit: $limit, offset: $offset) {\n\t\t\ttotal\n\t\t\tlimit\n\t\t\titems {\n\t\t\t\tid\n\t\t\t\tsubject\n\t\t\t\tlastMessageAt\n\t\t\t\tmessageCount\n\t\t\t\thasAttachments\n\t\t\t\tlatest {\n\t\t\t\t\tid\n\t\t\t\t\tfrom {\n\t\t\t\t\t\tname\n\t\t\t\t\t\temail\n\t\t\t\t\t}\n\t\t\t\t\tsnippet\n\t\t\t\t\tdirection\n\t\t\t\t\tencrypted\n\t\t\t\t\tsigned\n\t\t\t\t\tbodyState\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery EmailInbox($limit: Int, $offset: Int) {\n\t\temailThreads(limit: $limit, offset: $offset) {\n\t\t\ttotal\n\t\t\tlimit\n\t\t\titems {\n\t\t\t\tid\n\t\t\t\tsubject\n\t\t\t\tlastMessageAt\n\t\t\t\tmessageCount\n\t\t\t\thasAttachments\n\t\t\t\tlatest {\n\t\t\t\t\tid\n\t\t\t\t\tfrom {\n\t\t\t\t\t\tname\n\t\t\t\t\t\temail\n\t\t\t\t\t}\n\t\t\t\t\tsnippet\n\t\t\t\t\tdirection\n\t\t\t\t\tencrypted\n\t\t\t\t\tsigned\n\t\t\t\t\tbodyState\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;