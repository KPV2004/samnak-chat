// Hand-written M2 (chat) shapes and calls, from the plan's "Chat API" section.
// Replace with the generated schema once the chat module's OpenAPI is merged
// (checkpoint C), the way m1.ts did.
import type { ApiClient } from "./client";

export type ChannelType = "public" | "private" | "dm";

export interface Channel {
  id: string;
  type: ChannelType;
  name: string;
  topic: string;
  last_seq: number;
  last_read_seq: number;
  unread_count: number;
  mention_count: number;
  member_count: number;
  dm_user_ids: string[];
  archived_at: string | null;
  created_at: string;
}

export interface ChannelMember {
  user_id: string;
  display_name: string;
  joined_at: string;
}

export interface Attachment {
  id: string;
  file_name: string;
  size: number;
  mime_type: string;
  is_image: boolean;
}

export interface Reaction {
  emoji: string;
  count: number;
  me: boolean;
}

export interface Message {
  id: string;
  channel_id: string;
  seq: number;
  sender_id: string;
  client_msg_id: string;
  /** `text` or a card kind such as `approval.request`. */
  kind: string;
  body: string;
  payload: unknown;
  fallback_text: string;
  thread_root_id: string | null;
  reply_count: number;
  attachments: Attachment[];
  reactions: Reaction[];
  mentions: string[];
  mentions_channel: boolean;
  edited_at: string | null;
  deleted_at: string | null;
  created_at: string;
}

export interface MessagePage {
  items: Message[];
  has_more: boolean;
}

export interface ListMessagesParams {
  /** Catch-up: messages with a greater `seq`, ascending. */
  after_seq?: number;
  /** History: messages with a smaller `seq`. */
  before_seq?: number;
  /** At most 100. */
  limit?: number;
  thread_root_id?: string;
}

export interface SendMessageInput {
  client_msg_id: string;
  body: string;
  thread_root_id?: string;
  attachment_ids?: string[];
}

export interface CreateChannelInput {
  type: "public" | "private";
  name: string;
  topic?: string;
  member_ids?: string[];
}

export interface M2Api {
  listChannels(scope?: "joined" | "public"): Promise<{ items: Channel[] }>;
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
  addReaction(messageId: string, emoji: string): Promise<void>;
  removeReaction(messageId: string, emoji: string): Promise<void>;
}

const base = "/api/v1/chat";
const enc = encodeURIComponent;

type Query = Record<string, string | number | undefined>;

function call<T>(client: ApiClient, method: string, path: string, body?: unknown, query?: Query): Promise<T> {
  const qs = Object.entries(query ?? {})
    .filter(([, v]) => v !== undefined)
    .map(([k, v]) => `${k}=${enc(String(v))}`)
    .join("&");
  const init: Parameters<ApiClient["request"]>[1] = { method };
  if (body !== undefined) {
    init.headers = { "Content-Type": "application/json" };
    init.body = JSON.stringify(body);
  }
  return client.request(`${base}${path}${qs ? `?${qs}` : ""}`, init);
}

export function createM2Api(client: ApiClient): M2Api {
  return {
    listChannels: (scope) => call(client, "GET", "/channels", undefined, { scope }),
    createChannel: (input) => call(client, "POST", "/channels", input),
    openDm: (userIds) => call(client, "POST", "/dms", { user_ids: userIds }),
    getChannel: (id) => call(client, "GET", `/channels/${enc(id)}`),
    joinChannel: (id) => call(client, "POST", `/channels/${enc(id)}/join`),
    leaveChannel: (id) => call(client, "POST", `/channels/${enc(id)}/leave`),
    listChannelMembers: (id) => call(client, "GET", `/channels/${enc(id)}/members`),
    listMessages: (channelId, params = {}) =>
      call(client, "GET", `/channels/${enc(channelId)}/messages`, undefined, { ...params }),
    sendMessage: (channelId, input) => call(client, "POST", `/channels/${enc(channelId)}/messages`, input),
    editMessage: (id, body) => call(client, "PATCH", `/messages/${enc(id)}`, { body }),
    deleteMessage: (id) => call(client, "DELETE", `/messages/${enc(id)}`),
    markRead: (channelId, seq) => call(client, "POST", `/channels/${enc(channelId)}/read`, { seq }),
    addReaction: (messageId, emoji) => call(client, "POST", `/messages/${enc(messageId)}/reactions`, { emoji }),
    removeReaction: (messageId, emoji) =>
      call(client, "DELETE", `/messages/${enc(messageId)}/reactions/${enc(emoji)}`),
  };
}
