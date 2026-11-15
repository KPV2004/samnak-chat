// WebSocket frames, from the plan's "WebSocket protocol" and SRS 14.3.
// The plan fixes the type names and that every server event's `data` carries
// `workspace_id`; the payloads below are this package's reading of it and must
// match what the chat module sends (see the M2-04 report).
import type { Channel, Message } from "../api/m2";

export interface ReadyData {
  user_id: string;
  device_id: string;
}

interface InWorkspace {
  workspace_id: string;
}

/** Server → client. Each key is an envelope `type`; the value is its `data`. */
export interface ServerEventMap {
  ready: ReadyData;
  "message.created": InWorkspace & { message: Message };
  "message.updated": InWorkspace & { message: Message };
  /** The message keeps its row and `seq`; clients blank the body. */
  "message.deleted": InWorkspace & { channel_id: string; message_id: string; deleted_at?: string };
  "card.updated": InWorkspace & { channel_id: string; message_id: string; payload: unknown };
  /** Sent to the reader's own devices. */
  "channel.read": InWorkspace & { channel_id: string; seq: number };
  "channel.updated": InWorkspace & { channel: Channel };
  "presence.changed": InWorkspace & { user_id: string; online: boolean };
  typing: InWorkspace & { channel_id: string; user_id: string };
  "notification.created": InWorkspace & Record<string, unknown>;
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
]);
