import type { NotificationItem } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";
import type { T } from "../i18n/useT";

/**
 * The kinds the web knows how to say in the reader's language. Every other kind
 * comes from a module the web does not render, so it keeps the source module's
 * `title` and `body` (NOTI-06 fallback).
 *
 * The server's `data` carries ids (`channel_id`, `message_id`, `seq`,
 * `thread_root_id`), not display names, so the localised label names the kind
 * and `title` still says who and where.
 */
const knownKinds: Record<string, MessageKey> = {
  "chat.mention": "notify.kind.mention",
  "chat.dm": "notify.kind.dm",
  "chat.thread_reply": "notify.kind.threadReply",
};

/** A localised label for a kind the web knows; null for a kind it does not. */
export function notificationLabel(kind: string, t: T): string | null {
  const key = knownKinds[kind];
  return key ? t(key) : null;
}

/** Where a click goes: the server's link, else the chat channel carried in `data`. */
export function notificationLink(notification: NotificationItem): string {
  if (notification.link) return notification.link;
  const channel = notification.data?.channel_id;
  return typeof channel === "string" && channel ? `/chat/${channel}` : "";
}
