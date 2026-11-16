import { beforeEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "../api/client";
import { createM3Api, type NotificationItem } from "../api/m3";
import { createRealtimeClient } from "../realtime/client";
import { FakeSocket } from "../realtime/fake-socket";
import { FakeNotifyServer } from "./fake-server";
import { createNotificationStore } from "./store";

declare function setTimeout(fn: () => void, ms: number): unknown;

/** Lets every pending promise (fake fetch, store loops) run to completion. */
const settle = () => new Promise<void>((r) => setTimeout(r, 0));

function setup(opts: { workspaceId?: string | null; seed?: Partial<NotificationItem>[] } = {}) {
  const server = new FakeNotifyServer();
  for (const item of opts.seed ?? []) server.add(item);
  const api = createM3Api(createApiClient({ baseUrl: "", fetch: server.fetch }));
  FakeSocket.reset();
  const realtime = createRealtimeClient({
    url: "ws://x/ws",
    createSocket: (url) => new FakeSocket(url),
    getToken: () => "tok",
    minBackoffMs: 1,
    maxBackoffMs: 1,
    random: () => 0,
  });
  const store = createNotificationStore({
    api,
    realtime,
    getWorkspaceId: () => (opts.workspaceId === undefined ? "w1" : opts.workspaceId),
    pageSize: 2,
    now: () => new Date("2026-02-02T00:00:00Z"),
  });
  realtime.connect();
  const sock = FakeSocket.last();
  const ready = (s = FakeSocket.last()) => {
    s.open();
    s.receive("ready", { user_id: "u1", device_id: "d1" });
  };
  ready(sock);

  const emit = (type: string, data: object, s: FakeSocket = FakeSocket.last()) =>
    s.receive(type, { workspace_id: "w1", ...data });

  return { server, store, realtime, sock, ready, emit };
}

let t: ReturnType<typeof setup>;

beforeEach(() => {
  t = setup();
});

describe("count", () => {
  it("loads the unread count when the socket is ready", async () => {
    t = setup({ seed: [{}, {}] });
    await settle();
    expect(t.server.requests).toContain("GET /unread-count");
    expect(t.store.getState().unreadCount).toBe(2);
  });

  it("refetches the count after a reconnect", async () => {
    t = setup({ seed: [{}] });
    await settle();
    expect(t.store.getState().unreadCount).toBe(1);

    t.sock.drop();
    t.server.add();
    await vi.waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    t.ready();
    await settle();

    expect(t.store.getState().unreadCount).toBe(2);
  });

  it("keeps the old count when the request fails, and asks again on the next ready", async () => {
    t = setup({ seed: [{}] });
    await settle();
    t.server.failCounts = 1;
    t.server.add();
    t.sock.drop();
    await vi.waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    t.ready();
    await settle();
    expect(t.store.getState().unreadCount).toBe(1); // the failed answer changes nothing

    t.ready(); // the hub sends `ready` again after the next handshake
    await settle();
    expect(t.store.getState().unreadCount).toBe(2);
  });
});

describe("live events", () => {
  it("adds a notification it is sent and takes the server's count", async () => {
    const n = t.server.add();
    t.emit("notification.created", { notification: n, unread_count: 3 });
    expect(t.store.getState().items.map((i) => i.id)).toEqual([n.id]);
    expect(t.store.getState().unreadCount).toBe(3);
  });

  it("ignores a notification it already holds", async () => {
    const n = t.server.add();
    t.emit("notification.created", { notification: n, unread_count: 1 });
    t.emit("notification.created", { notification: n, unread_count: 1 });
    expect(t.store.getState().items).toHaveLength(1);
  });

  it("ignores events of another workspace", async () => {
    const n = t.server.add();
    FakeSocket.last().receive("notification.created", { workspace_id: "w2", notification: n, unread_count: 5 });
    expect(t.store.getState().items).toEqual([]);
    expect(t.store.getState().unreadCount).toBe(0);
  });

  it("applies a read event for named notifications, from another device", async () => {
    const a = t.server.add();
    const b = t.server.add();
    await t.store.load();
    t.emit("notification.read", { ids: [a.id], all: false, unread_count: 1 });
    const items = t.store.getState().items;
    expect(items.find((i) => i.id === a.id)?.read_at).not.toBeNull();
    expect(items.find((i) => i.id === b.id)?.read_at).toBeNull();
    expect(t.store.getState().unreadCount).toBe(1);
  });

  it("applies read-all from another device", async () => {
    t.server.add();
    t.server.add();
    await t.store.load();
    t.emit("notification.read", { ids: [], all: true, unread_count: 0 });
    expect(t.store.getState().items.every((i) => i.read_at !== null)).toBe(true);
    expect(t.store.getState().unreadCount).toBe(0);
  });

  it("runs nothing after dispose", () => {
    const n = t.server.add();
    t.store.dispose();
    t.emit("notification.created", { notification: n, unread_count: 4 });
    expect(t.store.getState().items).toEqual([]);
    expect(t.store.getState().unreadCount).toBe(0);
  });
});

describe("the list", () => {
  it("loads the newest page first and appends the next older page", async () => {
    t.server.add();
    t.server.add();
    t.server.add();
    await t.store.load();
    expect(t.store.getState()).toMatchObject({ hasMore: true, status: "ready" });
    expect(t.store.getState().items.map((i) => i.id)).toEqual(["n3", "n2"]);

    await t.store.loadMore();
    expect(t.store.getState().hasMore).toBe(false);
    expect(t.store.getState().items.map((i) => i.id)).toEqual(["n3", "n2", "n1"]);
    expect(t.server.requests).toContain("GET /notifications?before=n2&limit=2");
  });

  it("does nothing when there is no older page", async () => {
    await t.store.load();
    await t.store.loadMore();
    expect(t.server.requests.filter((r) => r.includes("before="))).toEqual([]);
  });

  it("reports a failed load", async () => {
    t.server.failLists = 1;
    await t.store.load();
    expect(t.store.getState().status).toBe("error");
  });

  it("marks one read at once, calls the API and lowers the count", async () => {
    t = setup({ seed: [{}, {}] });
    const n = t.server.items[0]!;
    await t.store.load();
    await t.store.markRead(n.id);
    const item = t.store.getState().items.find((i) => i.id === n.id);
    expect(item?.read_at).not.toBeNull();
    expect(t.store.getState().unreadCount).toBe(1);
    expect(t.server.requests).toContain(`POST /notifications/${n.id}/read`);
  });

  it("does not call the API twice for an already read notification", async () => {
    const n = t.server.add();
    await t.store.load();
    await t.store.markRead(n.id);
    await t.store.markRead(n.id);
    expect(t.server.requests.filter((r) => r.endsWith("/read"))).toHaveLength(1);
  });

  it("resyncs the count when a read call fails", async () => {
    t = setup({ seed: [{}, {}] });
    const n = t.server.items[0]!;
    await t.store.load();
    t.server.failReads = 1;
    await t.store.markRead(n.id);
    await settle();
    expect(t.server.requests.filter((r) => r === "GET /unread-count").length).toBeGreaterThan(1);
    expect(t.store.getState().unreadCount).toBe(2);
  });

  it("marks every notification read", async () => {
    t.server.add();
    t.server.add();
    await t.store.load();
    await t.store.markAllRead();
    expect(t.store.getState().items.every((i) => i.read_at !== null)).toBe(true);
    expect(t.store.getState().unreadCount).toBe(0);
    expect(t.server.requests).toContain("POST /read-all");
  });

  it("forgets everything on reset", async () => {
    t.server.add();
    await t.store.load();
    t.store.reset();
    expect(t.store.getState()).toMatchObject({ items: [], unreadCount: 0, hasMore: false, loaded: false });
  });
});
