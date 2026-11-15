// Pure helpers over one channel's state. Every function returns a new object;
// the store replaces its snapshot with the result so `useSyncExternalStore`
// sees a change.
import type { Channel, Message } from "../api/m2";

/** `sent` is confirmed by the server; the other two are local only. */
export type SendStatus = "sent" | "pending" | "failed";

export interface ChatMessage extends Message {
  status: SendStatus;
}

export interface ChannelState {
  channel: Channel;
  /** Confirmed messages sorted by `seq`, then pending and failed ones in send order. */
  messages: ChatMessage[];
  /** Highest `seq` known to be contiguous with everything before it. */
  lastSeq: number;
  lastReadSeq: number;
  /** Highest `seq` already reflected in `unreadCount`, so no message is counted twice. */
  countedSeq: number;
  unreadCount: number;
  mentionCount: number;
  /** `null` until history was loaded once. */
  hasMoreBefore: boolean | null;
  /**
   * Where the next older page starts (`before_seq`): the oldest `seq` of the pages
   * `loadHistory` fetched. Not the oldest message held, because thread replies
   * loaded on their own may be older than the channel history.
   */
  historyBefore: number | null;
}

/** `upToSeq` limits `lastSeq` for a channel first seen through an event that is newer than its page. */
export function newChannelState(channel: Channel, upToSeq: number = channel.last_seq): ChannelState {
  const lastSeq = Math.min(channel.last_seq, upToSeq);
  return {
    channel,
    messages: [],
    lastSeq,
    lastReadSeq: channel.last_read_seq,
    countedSeq: lastSeq,
    unreadCount: Math.max(0, Math.min(channel.unread_count, lastSeq - channel.last_read_seq)),
    mentionCount: channel.mention_count,
    hasMoreBefore: null,
    historyBefore: null,
  };
}

export function pendingMessage(channelId: string, userId: string, clientMsgId: string, now: Date, input: {
  body: string;
  thread_root_id?: string;
}): ChatMessage {
  return {
    id: clientMsgId,
    channel_id: channelId,
    seq: 0,
    sender_id: userId,
    client_msg_id: clientMsgId,
    kind: "text",
    body: input.body,
    payload: null,
    fallback_text: "",
    thread_root_id: input.thread_root_id ?? null,
    reply_count: 0,
    attachments: [],
    reactions: [],
    mentions: [],
    mentions_channel: false,
    edited_at: null,
    deleted_at: null,
    created_at: now.toISOString(),
    status: "pending",
  };
}

function sameMessage(a: Message, b: Message): boolean {
  return a.id === b.id || (a.client_msg_id !== "" && a.client_msg_id === b.client_msg_id && a.sender_id === b.sender_id);
}

export function find(ch: ChannelState, m: Message): ChatMessage | undefined {
  return ch.messages.find((x) => sameMessage(x, m));
}

/** Whether `incoming` may replace `existing`: a delete is final, an older edit never wins. */
function fresher(existing: Message, incoming: Message): boolean {
  if (existing.deleted_at) return false;
  if (incoming.deleted_at) return true;
  return (incoming.edited_at ?? "") >= (existing.edited_at ?? "");
}

/** Confirmed messages by `seq`; local ones keep their order after them (the sort is stable). */
function sorted(messages: ChatMessage[]): ChatMessage[] {
  const key = (m: ChatMessage) => (m.status === "sent" ? m.seq : Infinity);
  return [...messages].sort((a, b) => (key(a) === key(b) ? 0 : key(a) < key(b) ? -1 : 1));
}

/**
 * Adds a server message, or replaces the copy we hold (matched by `id`, or by
 * `client_msg_id` for our own pending one). A stale copy never overwrites a newer one.
 */
export function upsert(ch: ChannelState, m: Message): ChannelState {
  const i = ch.messages.findIndex((x) => sameMessage(x, m));
  const confirmed: ChatMessage = { ...m, status: "sent" };
  if (i < 0) return { ...ch, messages: sorted([...ch.messages, confirmed]) };
  const existing = ch.messages[i]!;
  if (existing.status === "sent" && !fresher(existing, m)) return ch;
  const messages = [...ch.messages];
  messages[i] = confirmed;
  return { ...ch, messages: sorted(messages) };
}

export function addLocal(ch: ChannelState, m: ChatMessage): ChannelState {
  return { ...ch, messages: sorted([...ch.messages, m]) };
}

export function mapMessage(ch: ChannelState, id: string, fn: (m: ChatMessage) => ChatMessage): ChannelState {
  const i = ch.messages.findIndex((m) => m.id === id);
  if (i < 0) return ch;
  const messages = [...ch.messages];
  messages[i] = fn(messages[i]!);
  return { ...ch, messages: sorted(messages) };
}

export function removeMessage(ch: ChannelState, id: string): ChannelState {
  return { ...ch, messages: ch.messages.filter((m) => m.id !== id) };
}

function mentionsMe(m: Message, me: string): boolean {
  return m.mentions_channel || m.mentions.includes(me);
}

/**
 * Moves `lastSeq` to `m.seq` and counts `m` as unread when it is new to the
 * counters, from someone else, and the channel is not open.
 */
export function account(ch: ChannelState, m: Message, me: string, channelOpen: boolean): ChannelState {
  const next = { ...ch, lastSeq: Math.max(ch.lastSeq, m.seq) };
  if (m.seq <= ch.countedSeq) return next;
  next.countedSeq = m.seq;
  if (m.sender_id !== me && !m.deleted_at && !channelOpen) {
    next.unreadCount += 1;
    if (mentionsMe(m, me)) next.mentionCount += 1;
  }
  return next;
}

/** Applies a read position and the counts the server reports for it. It never moves back. */
export function applyRead(
  ch: ChannelState,
  read: { last_read_seq: number; unread_count: number; mention_count: number },
): ChannelState {
  if (read.last_read_seq <= ch.lastReadSeq) return ch;
  return {
    ...ch,
    lastReadSeq: read.last_read_seq,
    unreadCount: read.unread_count,
    mentionCount: read.mention_count,
  };
}
