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
    "\n\tquery WhatsAppConversations {\n\t\twhatsAppConversations {\n\t\t\tid\n\t\t\tstatus\n\t\t\tlastActivityAt\n\t\t\tlastMessagePreview\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n": typeof types.WhatsAppConversationsDocument,
    "\n\tquery WhatsAppThread($conversationId: UUID!) {\n\t\twhatsAppConversation(id: $conversationId) {\n\t\t\tid\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t\tmessages {\n\t\t\t\t...ThreadMessage\n\t\t\t}\n\t\t}\n\t}\n": typeof types.WhatsAppThreadDocument,
    "\n\tfragment ThreadMessage on WhatsAppMessage {\n\t\tid\n\t\texternalId\n\t\tdirection\n\t\tcontent\n\t\tcontentType\n\t\tsentAt\n\t\tstatus\n\t\tstatusDetail\n\t\tmedia {\n\t\t\tstatus\n\t\t\tmimeType\n\t\t\tfilename\n\t\t\tfileSize\n\t\t\tvoice\n\t\t\tanimated\n\t\t\tdownloadPath\n\t\t}\n\t}\n": typeof types.ThreadMessageFragmentDoc,
    "\n\tmutation WhatsAppSendMessage($conversationId: UUID!, $content: String!) {\n\t\twhatsAppSendMessage(conversationId: $conversationId, content: $content) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n": typeof types.WhatsAppSendMessageDocument,
    "\n\tsubscription WhatsAppMessageReceived($conversationId: UUID!) {\n\t\twhatsAppMessageReceived(conversationId: $conversationId) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n": typeof types.WhatsAppMessageReceivedDocument,
};
const documents: Documents = {
    "\n\tquery WhatsAppConversations {\n\t\twhatsAppConversations {\n\t\t\tid\n\t\t\tstatus\n\t\t\tlastActivityAt\n\t\t\tlastMessagePreview\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n": types.WhatsAppConversationsDocument,
    "\n\tquery WhatsAppThread($conversationId: UUID!) {\n\t\twhatsAppConversation(id: $conversationId) {\n\t\t\tid\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t\tmessages {\n\t\t\t\t...ThreadMessage\n\t\t\t}\n\t\t}\n\t}\n": types.WhatsAppThreadDocument,
    "\n\tfragment ThreadMessage on WhatsAppMessage {\n\t\tid\n\t\texternalId\n\t\tdirection\n\t\tcontent\n\t\tcontentType\n\t\tsentAt\n\t\tstatus\n\t\tstatusDetail\n\t\tmedia {\n\t\t\tstatus\n\t\t\tmimeType\n\t\t\tfilename\n\t\t\tfileSize\n\t\t\tvoice\n\t\t\tanimated\n\t\t\tdownloadPath\n\t\t}\n\t}\n": types.ThreadMessageFragmentDoc,
    "\n\tmutation WhatsAppSendMessage($conversationId: UUID!, $content: String!) {\n\t\twhatsAppSendMessage(conversationId: $conversationId, content: $content) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n": types.WhatsAppSendMessageDocument,
    "\n\tsubscription WhatsAppMessageReceived($conversationId: UUID!) {\n\t\twhatsAppMessageReceived(conversationId: $conversationId) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n": types.WhatsAppMessageReceivedDocument,
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
export function graphql(source: "\n\tquery WhatsAppConversations {\n\t\twhatsAppConversations {\n\t\t\tid\n\t\t\tstatus\n\t\t\tlastActivityAt\n\t\t\tlastMessagePreview\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery WhatsAppConversations {\n\t\twhatsAppConversations {\n\t\t\tid\n\t\t\tstatus\n\t\t\tlastActivityAt\n\t\t\tlastMessagePreview\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery WhatsAppThread($conversationId: UUID!) {\n\t\twhatsAppConversation(id: $conversationId) {\n\t\t\tid\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t\tmessages {\n\t\t\t\t...ThreadMessage\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery WhatsAppThread($conversationId: UUID!) {\n\t\twhatsAppConversation(id: $conversationId) {\n\t\t\tid\n\t\t\tcontact {\n\t\t\t\tid\n\t\t\t\tname\n\t\t\t}\n\t\t\tmessages {\n\t\t\t\t...ThreadMessage\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tfragment ThreadMessage on WhatsAppMessage {\n\t\tid\n\t\texternalId\n\t\tdirection\n\t\tcontent\n\t\tcontentType\n\t\tsentAt\n\t\tstatus\n\t\tstatusDetail\n\t\tmedia {\n\t\t\tstatus\n\t\t\tmimeType\n\t\t\tfilename\n\t\t\tfileSize\n\t\t\tvoice\n\t\t\tanimated\n\t\t\tdownloadPath\n\t\t}\n\t}\n"): (typeof documents)["\n\tfragment ThreadMessage on WhatsAppMessage {\n\t\tid\n\t\texternalId\n\t\tdirection\n\t\tcontent\n\t\tcontentType\n\t\tsentAt\n\t\tstatus\n\t\tstatusDetail\n\t\tmedia {\n\t\t\tstatus\n\t\t\tmimeType\n\t\t\tfilename\n\t\t\tfileSize\n\t\t\tvoice\n\t\t\tanimated\n\t\t\tdownloadPath\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation WhatsAppSendMessage($conversationId: UUID!, $content: String!) {\n\t\twhatsAppSendMessage(conversationId: $conversationId, content: $content) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation WhatsAppSendMessage($conversationId: UUID!, $content: String!) {\n\t\twhatsAppSendMessage(conversationId: $conversationId, content: $content) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tsubscription WhatsAppMessageReceived($conversationId: UUID!) {\n\t\twhatsAppMessageReceived(conversationId: $conversationId) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n"): (typeof documents)["\n\tsubscription WhatsAppMessageReceived($conversationId: UUID!) {\n\t\twhatsAppMessageReceived(conversationId: $conversationId) {\n\t\t\t...ThreadMessage\n\t\t}\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;