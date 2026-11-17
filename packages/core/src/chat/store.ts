import type { M2Api, Message } from "../api/m2";
import type { RealtimeClient } from "../realtime/client";
import type { ServerEventMap } from "../realtime/events";
import {
  account,
  addLocal,
  find,
  mapMessage,
  newChannelState,
  newerPayload,
  applyRead,
  pendingMessage,
  removeMessage,
  upsert,
  type ChannelState,
} from "./channel";
import { uuidv7 } from "./uuid";

export type { ChannelState, ChatMessage, SendStatus } from "./channel";

/** Immutable snapshot, replaced on every change so it works with `useSyncExternalStore`. */
export interface ChatState {
  channels: Record<string, ChannelState>;
  /** Unread messages seen over the socket for workspaces that are not on screen. */
  workspaceUnread: Record<string, number>;
  activeChannelId: string | null;
}

export interface ChatStoreOptions {
  api: Pick<M2Api, "listChannels" | "getChannel" | "listMessages" | "sendMessage" | "markRead">;
  realtime: Pick<RealtimeClient, "on">;
  userId: string;
  /** The workspace on screen. Events of other workspaces only raise `workspaceUnread`. */
  getWorkspaceId?: () => string | null | undefined;
  newId?: () => string;
  now?: () => Date;
}

export interface SendOptions {
  thread_root_id?: string;
  attachment_ids?: string[];
}

export interface ChatStore {
  getState(): ChatState;
  subscribe(listener: () => void): () => void;
  /** Loads the joined channels and their counters; keeps messages already held. */
  loadChannels(): Promise<void>;
  /** Loads the newest page the first time, then the next older one. */
  loadHistory(channelId: string, limit?: number): Promise<void>;
  /** Loads every reply under a top-level message. They join the channel's messages. */
  loadThread(channelId: string, rootId: string): Promise<void>;
  /** Fetches `after_seq=` pages until `has_more` is false. Runs once at a time per channel. */
  catchUp(channelId: string): Promise<void>;
  /**
   * Shows the message at once as pending and posts it. `done` settles when the
   * attempt ended; it never rejects, a failure shows as `status: "failed"`.
   */
  send(channelId: string, body: string, options?: SendOptions): { clientMsgId: string; done: Promise<void> };
  /** Sends a failed message again with the same `client_msg_id`. */
  retry(channelId: string, clientMsgId: string): Promise<void>;
  /** Applies a message a REST call returned (an edit, a delete) as if its event had arrived. */
  applyMessage(message: Message): void;
  /** Drops a message that was not sent. */
  discard(channelId: string, clientMsgId: string): void;
  markRead(channelId: string): Promise<void>;
  /** The channel on screen: its new messages are not counted unread and are marked read. */
  setActiveChannel(channelId: string | null): Promise<void>;
  /** Forgets all channels, e.g. on a workspace switch. */
  reset(): void;
  /** Stops listening to the realtime client. */
  dispose(): void;
}

interface Draft extends SendOptions {
  channelId: string;
  body: string;
}

const noop = () => {};

export function createChatStore(opts: ChatStoreOptions): ChatStore {
  const { api, userId } = opts;
  const newId = opts.newId ?? uuidv7;
  const now = opts.now ?? (() => new Date());

  let state: ChatState = { channels: {}, workspaceUnread: {}, activeChannelId: null };
  const listeners = new Set<() => void>();
  const drafts = new Map<string, Draft>();
  const syncing = new Map<string, { rerun: boolean; promise: Promise<void> }>();
  const loadingChannel = new Map<string, Promise<void>>();
  const reading = new Map<string, Promise<void>>();
  const readAgain = new Set<string>();
  let disposed = false;
  // Bumped by `reset()`: an answer that was asked for before it must not fill the new state.
  let epoch = 0;

  function commit(next: ChatState) {
    state = next;
    listeners.forEach((l) => l());
  }

  function setChannel(id: string, next: ChannelState) {
    commit({ ...state, channels: { ...state.channels, [id]: next } });
  }

  /** Updates a channel we hold; does nothing for an unknown one. */
  function update(id: string, fn: (ch: ChannelState) => ChannelState) {
    const ch = state.channels[id];
    if (!ch) return;
    const next = fn(ch);
    if (next !== ch) setChannel(id, next);
  }

  const inView = (workspaceId: string) => {
    const current = opts.getWorkspaceId?.();
    return current == null || current === workspaceId;
  };

  // --- receiving ----------------------------------------------------------

  /** Registers a channel we first hear of; `upToSeq` keeps its newest message unapplied. */
  function ensureChannel(id: string, upToSeq: number): Promise<void> {
    if (state.channels[id]) return Promise.resolve();
    let loading = loadingChannel.get(id);
    if (!loading) {
      const asked = epoch;
      loading = api
        .getChannel(id)
        .then((c) => {
          if (asked === epoch && !state.channels[id]) setChannel(id, newChannelState(c, upToSeq));
        })
        .finally(() => loadingChannel.delete(id));
      loadingChannel.set(id, loading);
    }
    return loading;
  }

  /**
   * Applies one live message. Returns true when a seq is missing before it, so
   * the caller must catch up; the message itself is then left to the catch-up
   * (except our own, which confirms the pending copy).
   */
  function ingest(channelId: string, m: Message): boolean {
    const ch = state.channels[channelId];
    if (!ch) return false;
    const mine = m.sender_id === userId;
    if (m.seq <= ch.lastSeq) {
      // Inside the range we already know: only a message we hold can change.
      if (find(ch, m)) setChannel(channelId, upsert(ch, m));
      return false;
    }
    if (m.seq === ch.lastSeq + 1) {
      const open = state.activeChannelId === channelId;
      setChannel(channelId, account(upsert(ch, m), m, userId, open));
      if (open && !mine) void markRead(channelId);
      return false;
    }
    if (mine) setChannel(channelId, upsert(ch, m));
    return true;
  }

  async function onMessage(data: ServerEventMap["message.created"], created: boolean) {
    const m = data.message;
    if (!inView(data.workspace_id)) {
      if (created && m.sender_id !== userId) {
        const w = state.workspaceUnread;
        commit({ ...state, workspaceUnread: { ...w, [data.workspace_id]: (w[data.workspace_id] ?? 0) + 1 } });
      }
      return;
    }
    const learned = !state.channels[m.channel_id];
    if (learned) {
      try {
        await ensureChannel(m.channel_id, m.seq - 1);
      } catch {
        return; // the channel is not readable for us; nothing to show
      }
      if (disposed) return;
    }
    const gap = ingest(m.channel_id, m);
    const ch = state.channels[m.channel_id];
    // A channel first seen through an event may have newer messages than that event.
    if (gap || (learned && ch && ch.channel.last_seq > ch.lastSeq)) {
      void catchUp(m.channel_id).catch(noop);
    }
  }

  function applyFetched(channelId: string, items: Message[]) {
    update(channelId, (ch) => items.reduce((acc, m) => account(upsert(acc, m), m, userId, state.activeChannelId === channelId), ch));
  }

  async function pull(channelId: string) {
    const asked = epoch;
    for (;;) {
      const ch = state.channels[channelId];
      if (!ch || disposed) return;
      const page = await api.listMessages(channelId, { after_seq: ch.lastSeq, limit: 100 });
      if (asked !== epoch) return;
      applyFetched(channelId, page.items);
      if (!page.has_more || page.items.length === 0) break;
    }
    if (state.activeChannelId === channelId) void markRead(channelId);
  }

  function catchUp(channelId: string): Promise<void> {
    const running = syncing.get(channelId);
    if (running) {
      running.rerun = true;
      return running.promise;
    }
    const entry = { rerun: false, promise: Promise.resolve() };
    entry.promise = (async () => {
      try {
        do {
          entry.rerun = false;
          await pull(channelId);
        } while (entry.rerun);
      } finally {
        syncing.delete(channelId);
      }
    })();
    syncing.set(channelId, entry);
    return entry.promise;
  }

  const offs = [
    opts.realtime.on("ready", () => {
      // The socket never replays: ask for what was missed in every channel we hold.
      for (const id of Object.keys(state.channels)) void catchUp(id).catch(noop);
    }),
    opts.realtime.on("message.created", (d) => void onMessage(d, true)),
    opts.realtime.on("message.updated", (d) => void onMessage(d, false)),
    opts.realtime.on("message.deleted", (d) => void onMessage(d, false)),
    opts.realtime.on("card.updated", (d) => {
      if (!inView(d.workspace_id)) return;
      update(d.channel_id, (ch) => mapMessage(ch, d.message_id, (m) => ({ ...m, payload: newerPayload(m.payload, d.payload) })));
    }),
    opts.realtime.on("channel.read", (d) => {
      if (!inView(d.workspace_id)) return;
      update(d.channel_id, (ch) => applyRead(ch, d));
      // A read position beyond what we hold means messages we have not seen.
      const ch = state.channels[d.channel_id];
      if (ch && d.last_read_seq > ch.lastSeq) void catchUp(d.channel_id).catch(noop);
    }),
    opts.realtime.on("channel.updated", (d) => void onChannelUpdated(d)),
  ];

  async function onChannelUpdated(d: ServerEventMap["channel.updated"]) {
    if (!inView(d.workspace_id)) return;
    const held = state.channels[d.channel_id];
    if (!held) {
      // The event has no per-user counts, so a channel we do not know is fetched whole.
      try {
        await ensureChannel(d.channel_id, d.last_seq);
      } catch {
        return;
      }
      const fetched = state.channels[d.channel_id];
      if (fetched && !disposed && fetched.channel.last_seq > fetched.lastSeq) void catchUp(d.channel_id).catch(noop);
      return;
    }
    const channel = {
      ...held.channel,
      type: d.type,
      name: d.name,
      topic: d.topic,
      last_seq: d.last_seq,
      member_count: d.member_count,
      dm_user_ids: d.dm_user_ids,
      archived_at: d.archived_at,
      created_at: d.created_at,
    };
    setChannel(d.channel_id, { ...held, channel });
    if (d.last_seq > held.lastSeq) void catchUp(d.channel_id).catch(noop); // a message we have not seen
  }

  // --- reading state ------------------------------------------------------

  function markRead(channelId: string): Promise<void> {
    const ch = state.channels[channelId];
    if (!ch) return Promise.resolve();
    if (ch.lastReadSeq >= ch.lastSeq && ch.unreadCount === 0 && ch.mentionCount === 0) return Promise.resolve();
    setChannel(channelId, { ...ch, lastReadSeq: Math.max(ch.lastReadSeq, ch.lastSeq), unreadCount: 0, mentionCount: 0 });
    const inflight = reading.get(channelId);
    if (inflight) {
      readAgain.add(channelId); // post the newer position once the current one is done
      return inflight;
    }
    const run = (async () => {
      do {
        readAgain.delete(channelId);
        try {
          await api.markRead(channelId, state.channels[channelId]?.lastSeq ?? 0);
        } catch {
          // The position is re-sent with the next message; the server keeps the maximum.
        }
      } while (readAgain.has(channelId));
    })().finally(() => reading.delete(channelId));
    reading.set(channelId, run);
    return run;
  }

  // --- sending ------------------------------------------------------------

  async function post(clientMsgId: string): Promise<void> {
    const draft = drafts.get(clientMsgId);
    if (!draft) return;
    try {
      const m = await api.sendMessage(draft.channelId, {
        client_msg_id: clientMsgId,
        body: draft.body,
        ...(draft.thread_root_id ? { thread_root_id: draft.thread_root_id } : {}),
        ...(draft.attachment_ids ? { attachment_ids: draft.attachment_ids } : {}),
      });
      drafts.delete(clientMsgId);
      if (ingest(draft.channelId, m)) void catchUp(draft.channelId).catch(noop);
    } catch {
      // The echo may have confirmed it already; only an unconfirmed copy turns failed.
      update(draft.channelId, (ch) =>
        mapMessage(ch, clientMsgId, (m) => (m.status === "sent" ? m : { ...m, status: "failed" })),
      );
    }
  }

  // --- public API ---------------------------------------------------------

  return {
    getState: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },

    async loadChannels() {
      const asked = epoch;
      const { items } = await api.listChannels();
      if (asked !== epoch) return;
      const channels = { ...state.channels };
      const behind: string[] = [];
      for (const c of items) {
        const held = channels[c.id];
        if (!held) {
          channels[c.id] = newChannelState(c);
          continue;
        }
        channels[c.id] = {
          ...held,
          channel: c,
          lastReadSeq: c.last_read_seq,
          unreadCount: c.unread_count,
          mentionCount: c.mention_count,
          countedSeq: Math.max(held.countedSeq, c.last_seq),
        };
        if (c.last_seq > held.lastSeq) behind.push(c.id);
      }
      const workspaceUnread = { ...state.workspaceUnread };
      const current = opts.getWorkspaceId?.();
      if (current) delete workspaceUnread[current];
      commit({ ...state, channels, workspaceUnread });
      behind.forEach((id) => void catchUp(id).catch(noop));
    },

    async loadHistory(channelId, limit = 50) {
      const ch = state.channels[channelId];
      if (!ch || ch.hasMoreBefore === false) return;
      // The first page has no cursor: it is the newest one, whatever we know of last_seq.
      const before = ch.hasMoreBefore === null ? undefined : (ch.historyBefore ?? undefined);
      const asked = epoch;
      const page = await api.listMessages(channelId, { before_seq: before, limit });
      if (asked !== epoch) return;
      const seqs = page.items.map((m) => m.seq);
      update(channelId, (cur) => ({
        ...page.items.reduce(upsert, cur),
        hasMoreBefore: page.has_more,
        historyBefore: seqs.length > 0 ? Math.min(...seqs) : cur.historyBefore,
      }));
      // A page can reach past what the channel list said; the gap between is fetched forward.
      const now = state.channels[channelId];
      if (now && seqs.some((q) => q > now.lastSeq)) void catchUp(channelId).catch(noop);
    },

    async loadThread(channelId, rootId) {
      const asked = epoch;
      let before: number | undefined;
      for (;;) {
        const page = await api.listMessages(channelId, { thread_root_id: rootId, before_seq: before, limit: 100 });
        if (asked !== epoch) return;
        update(channelId, (cur) => page.items.reduce(upsert, cur));
        const oldest = Math.min(...page.items.map((m) => m.seq));
        if (!page.has_more || page.items.length === 0 || (before !== undefined && oldest >= before)) return;
        before = oldest;
      }
    },

    catchUp,

    send(channelId, body, options = {}) {
      const ch = state.channels[channelId];
      if (!ch) throw new Error(`unknown channel ${channelId}`);
      const clientMsgId = newId();
      drafts.set(clientMsgId, { channelId, body, ...options });
      setChannel(channelId, addLocal(ch, pendingMessage(channelId, userId, clientMsgId, now(), { body, ...options })));
      return { clientMsgId, done: post(clientMsgId) };
    },

    retry(channelId, clientMsgId) {
      const ch = state.channels[channelId];
      const failed = ch?.messages.find((m) => m.id === clientMsgId && m.status === "failed");
      if (!ch || !failed || !drafts.has(clientMsgId)) return Promise.resolve();
      setChannel(channelId, mapMessage(ch, clientMsgId, (m) => ({ ...m, status: "pending" })));
      return post(clientMsgId);
    },

    applyMessage(message) {
      void onMessage(
        { workspace_id: opts.getWorkspaceId?.() ?? "", channel_id: message.channel_id, message },
        false,
      );
    },

    discard(channelId, clientMsgId) {
      const ch = state.channels[channelId];
      const m = ch?.messages.find((x) => x.id === clientMsgId);
      if (!ch || !m || m.status === "sent") return;
      drafts.delete(clientMsgId);
      setChannel(channelId, removeMessage(ch, clientMsgId));
    },

    markRead,

    setActiveChannel(channelId) {
      commit({ ...state, activeChannelId: channelId });
      return channelId ? markRead(channelId) : Promise.resolve();
    },

    reset() {
      epoch++;
      drafts.clear();
      commit({ channels: {}, workspaceUnread: {}, activeChannelId: null });
    },

    dispose() {
      disposed = true;
      offs.forEach((off) => off());
    },
  };
}
