import type { NotificationItem } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";
import type { T } from "../i18n/useT";

/**
 * The kinds the web knows how to say in the reader's language. Every other kind
 * comes from a module the web does not render, so it keeps the source module's
 * `title` and `body` (NOTI-06 fallback).
 *
 * Chat's `data` carries `sender_name` and `channel_name` ("" for a DM), so
 * the web says who and where in the reader's language too.
 */
const knownKinds: Record<string, MessageKey> = {
  "chat.mention": "notify.kind.mention",
  "chat.dm": "notify.kind.dm",
  "chat.thread_reply": "notify.kind.threadReply",
};

/**
 * Approval notices have no `sender_name` in `data`, only ids, so the web says what happened
 * without a name instead of showing the server's English sentence (BT-5). The summary stays in `body`.
 */
const approvalTitles: Record<string, MessageKey> = {
  "approval.requested": "notify.approval.requested",
  "approval.approved": "notify.approval.approved",
  "approval.rejected": "notify.approval.rejected",
  "approval.cancelled": "notify.approval.cancelled",
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

/**
 * Who and where in the reader's language for chat kinds; else the server's `title`. Approval kinds
 * carry only `requester_id`, so a caller that can name people passes `nameOf` to say who asked.
 */
export function notificationTitle(notification: NotificationItem, t: T, nameOf?: (id: string) => string): string {
  const approvalKey = approvalTitles[notification.kind];
  if (approvalKey) {
    const requester = notification.data?.requester_id;
    const name = typeof requester === "string" && requester ? nameOf?.(requester) : undefined;
    if (notification.kind === "approval.requested" && name) return t("notify.approval.requestedBy", { name });
    return t(approvalKey);
  }
  const sender = notification.data?.sender_name;
  if (!knownKinds[notification.kind] || typeof sender !== "string" || !sender) return notification.title;
  const channel = notification.data?.channel_name;
  return typeof channel === "string" && channel
    ? t("notify.where.channel", { sender, channel })
    : t("notify.where.dm", { sender });
}
