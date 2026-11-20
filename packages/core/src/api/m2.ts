// M2 (chat) types come from the server's OpenAPI document (npm run gen:api);
// this file re-exports them under stable names and holds the typed calls, the
// way m1.ts does. A call is typed by the generated operation for its path, so
// reshaping an endpoint on the server makes `npm run build` fail here.
//
// Reactions and attachments are not in the document yet (M2-05 adds them), so
// their types and calls are written by hand below until `make openapi` brings
// them in; then replace them like the rest.
import type { ApiClient } from "./client";
import type { components, paths } from "./schema";

type Schemas = components["schemas"];

export type Channel = Schemas["Channel"];
/** The newest message of a channel as the list shows it; `null` on a channel with no message. */
export type LastMessage = NonNullable<Channel["last_message"]>;
export type ChannelType = Channel["type"];
export type ChannelMember = Schemas["ChannelMember"];
export type Attachment = Schemas["Attachment"];
export type Reaction = Schemas["Reaction"];
/** The document types `payload` as an empty object (it is a raw JSON card payload); it is `unknown` here. */
export type Message = Omit<Schemas["Message"], "payload"> & { payload: unknown };

export type MessagePage = Omit<Schemas["ListMessagesOutputBody"], "items"> & { items: Message[] };
export type ListMessagesParams = NonNullable<paths["/chat/channels/{id}/messages"]["get"]["parameters"]["query"]>;
export type ChannelScope = NonNullable<NonNullable<paths["/chat/channels"]["get"]["parameters"]["query"]>["scope"]>;

/** `attachment_ids` is added by M2-05; it is not in the document yet. */
export type SendMessageInput = Schemas["SendMessageInputBody"] & { attachment_ids?: string[] };
export type CreateChannelInput = Schemas["CreateChannelInputBody"];

type Method = "get" | "post" | "put" | "patch" | "delete";
type Operation<P extends keyof paths, M extends Method> = Exclude<paths[P][M], undefined>;
/** The methods a path really has: the generator marks the others `?: never`. */
type MethodsOf<P extends keyof paths> = { [M in Method]: [Operation<P, M>] extends [never] ? never : M }[Method];

type JsonResponse<O> = O extends { responses: { 200: { content: { "application/json": infer R } } } }
  ? R
  : O extends { responses: { 201: { content: { "application/json": infer R } } } }
    ? R
    : void;

interface CallOptions {
  body?: unknown;
  params?: Record<string, string>;
  query?: Record<string, string | number | undefined>;
}

const base = "/api/v1";
const enc = encodeURIComponent;

// Unlike m1's `call`, a query value of 0 is sent: `after_seq=0` means "from the start".
function request<T>(client: ApiClient, method: string, path: string, opts: CallOptions = {}): Promise<T> {
  let url = base + path.replace(/\{(\w+)\}/g, (_, name: string) => enc(opts.params?.[name] ?? ""));
  const qs = Object.entries(opts.query ?? {})
    .filter(([, v]) => v !== undefined)
    .map(([k, v]) => `${k}=${enc(String(v))}`)
    .join("&");
  if (qs) url += `?${qs}`;
  const init: Parameters<ApiClient["request"]>[1] = { method: method.toUpperCase() };
  if (opts.body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(opts.body);
  }
  return client.request(url, init);
}

/** Calls an operation by the key it has in the generated `paths`. */
function call<P extends keyof paths, M extends MethodsOf<P>>(
  client: ApiClient,
  method: M,
  path: P,
  opts?: CallOptions,
): Promise<JsonResponse<Operation<P, M>>> {
  return request(client, method, path as string, opts);
}

export interface M2Api {
  listChannels(scope?: ChannelScope): Promise<{ items: Channel[] }>;
  createChannel(input: CreateChannelInput): Promise<Channel>;
  openDm(userIds: string[]): Promise<Channel>;
  getChannel(id: string): Promise<Channel>;
  joinChannel(id: string): Promise<void>;
  leaveChannel(id: string): Promise<void>;
  listChannelMembers(id: string): Promise<{ items: ChannelMember[] }>;
  listMessages(channelId: string, params?: ListMessagesParams): Promise<MessagePage>;
  /** 201 for a new message, 200 with the original for a repeated `client_msg_id`. */
  sendMessage(channelId: string, input: SendMessageInput): Promise<Message>;
  editMessage(id: string, body: string): Promise<Message>;
  deleteMessage(id: string): Promise<void>;
  markRead(channelId: string, seq: number): Promise<void>;
  /** Pins the channel to the top of my list; 409 `conflict` after 30 pins. Idempotent. */
  pinChannel(id: string): Promise<void>;
  unpinChannel(id: string): Promise<void>;
  /** Hand-written until M2-05's endpoints are in the document. */
  addReaction(messageId: string, emoji: string): Promise<void>;
  /** Hand-written until M2-05's endpoints are in the document. */
  removeReaction(messageId: string, emoji: string): Promise<void>;
}

export function createM2Api(client: ApiClient): M2Api {
  return {
    listChannels: (scope) => call(client, "get", "/chat/channels", { query: { scope } }),
    createChannel: (input) => call(client, "post", "/chat/channels", { body: input }),
    openDm: (userIds) => call(client, "post", "/chat/dms", { body: { user_ids: userIds } }),
    getChannel: (id) => call(client, "get", "/chat/channels/{id}", { params: { id } }),
    joinChannel: (id) => call(client, "post", "/chat/channels/{id}/join", { params: { id } }),
    leaveChannel: (id) => call(client, "post", "/chat/channels/{id}/leave", { params: { id } }),
    listChannelMembers: (id) => call(client, "get", "/chat/channels/{id}/members", { params: { id } }),
    listMessages: (id, params = {}) =>
      call(client, "get", "/chat/channels/{id}/messages", { params: { id }, query: { ...params } }) as Promise<MessagePage>,
    sendMessage: (id, input) =>
      call(client, "post", "/chat/channels/{id}/messages", { params: { id }, body: input }) as Promise<Message>,
    editMessage: (id, body) =>
      call(client, "patch", "/chat/messages/{id}", { params: { id }, body: { body } }) as Promise<Message>,
    deleteMessage: (id) => call(client, "delete", "/chat/messages/{id}", { params: { id } }),
    pinChannel: (id) => call(client, "put", "/chat/channels/{id}/pin", { params: { id } }),
    unpinChannel: (id) => call(client, "delete", "/chat/channels/{id}/pin", { params: { id } }),
    markRead: (id, seq) => call(client, "post", "/chat/channels/{id}/read", { params: { id }, body: { seq } }),
    addReaction: (messageId, emoji) =>
      request(client, "post", "/chat/messages/{id}/reactions", { params: { id: messageId }, body: { emoji } }),
    removeReaction: (messageId, emoji) =>
      request(client, "delete", "/chat/messages/{id}/reactions/{emoji}", { params: { id: messageId, emoji } }),
  };
}
