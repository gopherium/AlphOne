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
    "\n\tquery Me {\n\t\tme {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t}\n\t}\n": typeof types.MeDocument,
    "\n\tmutation Login($email: String!, $password: String!) {\n\t\tlogin(email: $email, password: $password) {\n\t\t\tme {\n\t\t\t\tid\n\t\t\t\temail\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n": typeof types.LoginDocument,
    "\n\tmutation Logout {\n\t\tlogout\n\t}\n": typeof types.LogoutDocument,
    "\n\tquery Users {\n\t\tusers {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n": typeof types.UsersDocument,
    "\n\tmutation CreateUser($email: String!, $name: String!, $password: String!) {\n\t\tcreateUser(email: $email, name: $name, password: $password) {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n": typeof types.CreateUserDocument,
    "\n\tmutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {\n\t\tsetUserDisabled(id: $id, disabled: $disabled)\n\t}\n": typeof types.SetUserDisabledDocument,
    "\n\tquery ContactDetail($id: UUID!, $first: Int, $after: String) {\n\t\tcontact(id: $id) {\n\t\t\tid\n\t\t\tname\n\t\t\tcreatedAt\n\t\t\tidentities {\n\t\t\t\tid\n\t\t\t\tchannel\n\t\t\t\tidentifier\n\t\t\t\tdisplayName\n\t\t\t}\n\t\t\ttasks(status: \"open\", first: $first, after: $after) {\n\t\t\t\tedges {\n\t\t\t\t\tnode {\n\t\t\t\t\t\tid\n\t\t\t\t\t\ttitle\n\t\t\t\t\t\tstatus\n\t\t\t\t\t\tpriority\n\t\t\t\t\t\tdueOn\n\t\t\t\t\t}\n\t\t\t\t\tcursor\n\t\t\t\t}\n\t\t\t\tpageInfo {\n\t\t\t\t\thasNextPage\n\t\t\t\t\tendCursor\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n": typeof types.ContactDetailDocument,
    "\n\tquery Contacts($q: String, $first: Int, $after: String) {\n\t\tcontacts(q: $q, first: $first, after: $after) {\n\t\t\tedges {\n\t\t\t\tnode {\n\t\t\t\t\tid\n\t\t\t\t\tname\n\t\t\t\t\tcreatedAt\n\t\t\t\t}\n\t\t\t\tcursor\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\thasNextPage\n\t\t\t\tendCursor\n\t\t\t}\n\t\t}\n\t}\n": typeof types.ContactsDocument,
    "\n\tquery Version {\n\t\tversion\n\t}\n": typeof types.VersionDocument,
};
const documents: Documents = {
    "\n\tquery Me {\n\t\tme {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t}\n\t}\n": types.MeDocument,
    "\n\tmutation Login($email: String!, $password: String!) {\n\t\tlogin(email: $email, password: $password) {\n\t\t\tme {\n\t\t\t\tid\n\t\t\t\temail\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n": types.LoginDocument,
    "\n\tmutation Logout {\n\t\tlogout\n\t}\n": types.LogoutDocument,
    "\n\tquery Users {\n\t\tusers {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n": types.UsersDocument,
    "\n\tmutation CreateUser($email: String!, $name: String!, $password: String!) {\n\t\tcreateUser(email: $email, name: $name, password: $password) {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n": types.CreateUserDocument,
    "\n\tmutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {\n\t\tsetUserDisabled(id: $id, disabled: $disabled)\n\t}\n": types.SetUserDisabledDocument,
    "\n\tquery ContactDetail($id: UUID!, $first: Int, $after: String) {\n\t\tcontact(id: $id) {\n\t\t\tid\n\t\t\tname\n\t\t\tcreatedAt\n\t\t\tidentities {\n\t\t\t\tid\n\t\t\t\tchannel\n\t\t\t\tidentifier\n\t\t\t\tdisplayName\n\t\t\t}\n\t\t\ttasks(status: \"open\", first: $first, after: $after) {\n\t\t\t\tedges {\n\t\t\t\t\tnode {\n\t\t\t\t\t\tid\n\t\t\t\t\t\ttitle\n\t\t\t\t\t\tstatus\n\t\t\t\t\t\tpriority\n\t\t\t\t\t\tdueOn\n\t\t\t\t\t}\n\t\t\t\t\tcursor\n\t\t\t\t}\n\t\t\t\tpageInfo {\n\t\t\t\t\thasNextPage\n\t\t\t\t\tendCursor\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n": types.ContactDetailDocument,
    "\n\tquery Contacts($q: String, $first: Int, $after: String) {\n\t\tcontacts(q: $q, first: $first, after: $after) {\n\t\t\tedges {\n\t\t\t\tnode {\n\t\t\t\t\tid\n\t\t\t\t\tname\n\t\t\t\t\tcreatedAt\n\t\t\t\t}\n\t\t\t\tcursor\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\thasNextPage\n\t\t\t\tendCursor\n\t\t\t}\n\t\t}\n\t}\n": types.ContactsDocument,
    "\n\tquery Version {\n\t\tversion\n\t}\n": types.VersionDocument,
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
export function graphql(source: "\n\tquery Me {\n\t\tme {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Me {\n\t\tme {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation Login($email: String!, $password: String!) {\n\t\tlogin(email: $email, password: $password) {\n\t\t\tme {\n\t\t\t\tid\n\t\t\t\temail\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation Login($email: String!, $password: String!) {\n\t\tlogin(email: $email, password: $password) {\n\t\t\tme {\n\t\t\t\tid\n\t\t\t\temail\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation Logout {\n\t\tlogout\n\t}\n"): (typeof documents)["\n\tmutation Logout {\n\t\tlogout\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery Users {\n\t\tusers {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Users {\n\t\tusers {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation CreateUser($email: String!, $name: String!, $password: String!) {\n\t\tcreateUser(email: $email, name: $name, password: $password) {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation CreateUser($email: String!, $name: String!, $password: String!) {\n\t\tcreateUser(email: $email, name: $name, password: $password) {\n\t\t\tid\n\t\t\temail\n\t\t\tname\n\t\t\tdisabled\n\t\t\tcreatedAt\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {\n\t\tsetUserDisabled(id: $id, disabled: $disabled)\n\t}\n"): (typeof documents)["\n\tmutation SetUserDisabled($id: UUID!, $disabled: Boolean!) {\n\t\tsetUserDisabled(id: $id, disabled: $disabled)\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery ContactDetail($id: UUID!, $first: Int, $after: String) {\n\t\tcontact(id: $id) {\n\t\t\tid\n\t\t\tname\n\t\t\tcreatedAt\n\t\t\tidentities {\n\t\t\t\tid\n\t\t\t\tchannel\n\t\t\t\tidentifier\n\t\t\t\tdisplayName\n\t\t\t}\n\t\t\ttasks(status: \"open\", first: $first, after: $after) {\n\t\t\t\tedges {\n\t\t\t\t\tnode {\n\t\t\t\t\t\tid\n\t\t\t\t\t\ttitle\n\t\t\t\t\t\tstatus\n\t\t\t\t\t\tpriority\n\t\t\t\t\t\tdueOn\n\t\t\t\t\t}\n\t\t\t\t\tcursor\n\t\t\t\t}\n\t\t\t\tpageInfo {\n\t\t\t\t\thasNextPage\n\t\t\t\t\tendCursor\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery ContactDetail($id: UUID!, $first: Int, $after: String) {\n\t\tcontact(id: $id) {\n\t\t\tid\n\t\t\tname\n\t\t\tcreatedAt\n\t\t\tidentities {\n\t\t\t\tid\n\t\t\t\tchannel\n\t\t\t\tidentifier\n\t\t\t\tdisplayName\n\t\t\t}\n\t\t\ttasks(status: \"open\", first: $first, after: $after) {\n\t\t\t\tedges {\n\t\t\t\t\tnode {\n\t\t\t\t\t\tid\n\t\t\t\t\t\ttitle\n\t\t\t\t\t\tstatus\n\t\t\t\t\t\tpriority\n\t\t\t\t\t\tdueOn\n\t\t\t\t\t}\n\t\t\t\t\tcursor\n\t\t\t\t}\n\t\t\t\tpageInfo {\n\t\t\t\t\thasNextPage\n\t\t\t\t\tendCursor\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery Contacts($q: String, $first: Int, $after: String) {\n\t\tcontacts(q: $q, first: $first, after: $after) {\n\t\t\tedges {\n\t\t\t\tnode {\n\t\t\t\t\tid\n\t\t\t\t\tname\n\t\t\t\t\tcreatedAt\n\t\t\t\t}\n\t\t\t\tcursor\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\thasNextPage\n\t\t\t\tendCursor\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Contacts($q: String, $first: Int, $after: String) {\n\t\tcontacts(q: $q, first: $first, after: $after) {\n\t\t\tedges {\n\t\t\t\tnode {\n\t\t\t\t\tid\n\t\t\t\t\tname\n\t\t\t\t\tcreatedAt\n\t\t\t\t}\n\t\t\t\tcursor\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\thasNextPage\n\t\t\t\tendCursor\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery Version {\n\t\tversion\n\t}\n"): (typeof documents)["\n\tquery Version {\n\t\tversion\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;