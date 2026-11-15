// WebSocket frames. The shapes are the ones the server sends, fixed in the
// plan's "Event payloads" (chat/adapter/realtime). Every event of a workspace
// carries `workspace_id`; `presence.changed` is the exception.
import type { Channel, Message } from "../api/m2";

export interface ReadyData {
  user_id: string;
  device_id: string;
}

interface InWorkspace {
  workspace_id: string;
}

interface InChannel extends InWorkspace {
  channel_id: string;
}

/** Server → client. Each key is an envelope `type`; the value is its `data`. */
export interface ServerEventMap {
  ready: ReadyData;
  "message.created": InChannel & { message: Message };
  "message.updated": InChannel & { message: Message };
  /** The message keeps its row and `seq`; `message` has an empty body and `deleted_at` set. */
  "message.deleted": InChannel & { message: Message };
  "card.updated": InChannel & { message_id: string; payload: unknown };
  /** Sent to the reader's own devices; the counts are the server's. */
  "channel.read": InChannel & { last_read_seq: number; unread_count: number; mention_count: number };
  /** Flat, and without the per-user counts of a `Channel`. */
  "channel.updated": InChannel &
    Pick<Channel, "type" | "name" | "topic" | "last_seq" | "member_count" | "dm_user_ids" | "archived_at" | "created_at">;
  "presence.changed": { user_id: string; online: boolean };
  typing: InChannel & { user_id: string };
  "notification.created": InWorkspace & Record<string, unknown>;
  /** The hub's answer to a bad client frame; the connection stays open. */
  error: { type: string; code: string; message: string };
}

export type ServerEventType = keyof ServerEventMap;

/** Client → server (`auth` and `ping` are sent by the client itself). */
export interface ClientEventMap {
  typing: { channel_id: string };
}

export const serverEventTypes: ReadonlySet<string> = new Set<ServerEventType>([
  "ready",
  "message.created",
  "message.updated",
  "message.deleted",
  "card.updated",
  "channel.read",
  "channel.updated",
  "presence.changed",
  "typing",
  "notification.created",
  "error",
]);
