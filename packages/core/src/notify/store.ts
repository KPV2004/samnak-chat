import type { M3Api, NotificationItem } from "../api/m3";
import type { RealtimeClient } from "../realtime/client";
import type { ServerEventMap } from "../realtime/events";

/** Immutable snapshot, replaced on every change so it works with `useSyncExternalStore`. */
export interface NotificationState {
  unreadCount: number;
  /** Newest first, as the API returns them. */
  items: NotificationItem[];
  hasMore: boolean;
  /** The newest page has been asked for at least once. */
  loaded: boolean;
  status: "idle" | "loading" | "ready" | "error";
}

export interface NotificationStoreOptions {
  api: Pick<M3Api, "listNotifications" | "unreadNotificationCount" | "readNotification" | "readAllNotifications">;
  realtime: Pick<RealtimeClient, "on">;
  /** The workspace on screen. Events of other workspaces are ignored. */
  getWorkspaceId?: () => string | null | undefined;
  /** Items per page; the server caps it at 50. */
  pageSize?: number;
  now?: () => Date;
}

export interface NotificationStore {
  getState(): NotificationState;
  subscribe(listener: () => void): () => void;
  /**
   * Asks the server for the unread count. It also runs on every `ready`, so a
   * reconnect resyncs the bell (NOTI-03).
   */
  loadCount(): Promise<void>;
  /** Loads the newest page. The panel calls it when it opens. */
  load(): Promise<void>;
  /** Appends the next older page. */
  loadMore(): Promise<void>;
  /** Marks one read at once, then tells the server (NOTI-04). */
  markRead(id: string): Promise<void>;
  markAllRead(): Promise<void>;
  /** Forgets everything, e.g. on a workspace switch. */
  reset(): void;
  /** Stops listening to the realtime client. */
  dispose(): void;
}

const initial: NotificationState = { unreadCount: 0, items: [], hasMore: false, loaded: false, status: "idle" };

export function createNotificationStore(opts: NotificationStoreOptions): NotificationStore {
  const pageSize = opts.pageSize ?? 20;
  const now = opts.now ?? (() => new Date());

  let state: NotificationState = initial;
  const listeners = new Set<() => void>();
  let disposed = false;
  // Bumped by `reset()`: an answer asked for before it must not fill the new state.
  let epoch = 0;
  let counting: Promise<void> | null = null;

  function commit(next: NotificationState) {
    state = next;
    listeners.forEach((l) => l());
  }

  const inView = (workspaceId: string) => {
    const current = opts.getWorkspaceId?.();
    return current == null || current === workspaceId;
  };

  const readAt = () => now().toISOString();

  function mergeRead(items: NotificationItem[], ids: Set<string>): NotificationItem[] {
    const at = readAt();
    return items.map((n) => (!n.read_at && ids.has(n.id) ? { ...n, read_at: at } : n));
  }

  function loadCount(): Promise<void> {
    if (counting) return counting;
    // The socket can be ready before a workspace is on screen; the count waits for it.
    if (opts.getWorkspaceId && opts.getWorkspaceId() == null) return Promise.resolve();
    const asked = epoch;
    counting = (async () => {
      try {
        const { count } = await opts.api.unreadNotificationCount();
        if (asked === epoch && !disposed) commit({ ...state, unreadCount: count });
      } catch {
        // The next `ready` or a fresh open asks again; the last known count stays.
      } finally {
        counting = null;
      }
    })();
    return counting;
  }

  function load(): Promise<void> {
    const asked = epoch;
    if (!state.loaded) commit({ ...state, status: "loading" });
    return opts.api.listNotifications({ limit: pageSize }).then(
      (page) => {
        if (asked !== epoch || disposed) return;
        commit({ ...state, items: page.items, hasMore: page.has_more, loaded: true, status: "ready" });
      },
      () => {
        if (asked === epoch && !disposed) commit({ ...state, status: "error" });
      },
    );
  }

  function loadMore(): Promise<void> {
    const before = state.items.at(-1)?.id;
    if (!state.hasMore || !before) return load();
    const asked = epoch;
    return opts.api.listNotifications({ before, limit: pageSize }).then(
      (page) => {
        if (asked !== epoch || disposed) return;
        const held = new Set(state.items.map((n) => n.id));
        const items = [...state.items, ...page.items.filter((n) => !held.has(n.id))];
        commit({ ...state, items, hasMore: page.has_more });
      },
      () => {
        // Keep what we have; the button stays for another try.
      },
    );
  }

  function markRead(id: string): Promise<void> {
    const item = state.items.find((n) => n.id === id);
    if (!item || item.read_at) return Promise.resolve();
    commit({
      ...state,
      items: mergeRead(state.items, new Set([id])),
      unreadCount: Math.max(0, state.unreadCount - 1),
    });
    return opts.api.readNotification(id).catch(() => loadCount().then(() => undefined));
  }

  function markAllRead(): Promise<void> {
    if (state.unreadCount === 0 && state.items.every((n) => n.read_at)) return Promise.resolve();
    const at = readAt();
    commit({
      ...state,
      items: state.items.map((n) => (n.read_at ? n : { ...n, read_at: at })),
      unreadCount: 0,
    });
    return opts.api.readAllNotifications().catch(() => loadCount().then(() => undefined));
  }

  const offs = [
    opts.realtime.on("ready", () => {
      // The socket replays nothing: the count (and the open list) must come from the API.
      void loadCount();
      if (state.loaded) void load();
    }),
    opts.realtime.on("notification.created", (d: ServerEventMap["notification.created"]) => {
      if (!inView(d.workspace_id)) return;
      const n = d.notification;
      const items = state.items.some((i) => i.id === n.id) ? state.items : [n, ...state.items];
      commit({ ...state, items, unreadCount: d.unread_count });
    }),
    opts.realtime.on("notification.read", (d: ServerEventMap["notification.read"]) => {
      if (!inView(d.workspace_id)) return;
      const at = readAt();
      const items = d.all
        ? state.items.map((n) => (n.read_at ? n : { ...n, read_at: at }))
        : mergeRead(state.items, new Set(d.ids));
      commit({ ...state, items, unreadCount: d.unread_count });
    }),
  ];

  return {
    getState: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    loadCount,
    load,
    loadMore,
    markRead,
    markAllRead,
    reset() {
      epoch++;
      counting = null;
      commit(initial);
    },
    dispose() {
      disposed = true;
      offs.forEach((off) => off());
    },
  };
}
