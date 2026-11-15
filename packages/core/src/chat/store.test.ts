import { beforeEach, describe, expect, it, vi } from "vitest";
import { createApiClient } from "../api/client";
import { createM2Api, type Message } from "../api/m2";
import { createRealtimeClient } from "../realtime/client";
import { FakeSocket } from "../realtime/fake-socket";
import { FakeChatServer } from "./fake-server";
import { createChatStore } from "./store";

declare function setTimeout(fn: () => void, ms: number): unknown;

/** Lets every pending promise (fake fetch, store loops) run to completion. */
const settle = () => new Promise<void>((r) => setTimeout(r, 0));

function setup(opts: { workspaceId?: string | null } = {}) {
  const server = new FakeChatServer();
  const api = createM2Api(createApiClient({ baseUrl: "", fetch: server.fetch }));
  FakeSocket.reset();
  const realtime = createRealtimeClient({
    url: "ws://x/ws",
    createSocket: (url) => new FakeSocket(url),
    getToken: () => "tok",
    minBackoffMs: 1,
    maxBackoffMs: 1,
    random: () => 0,
  });
  let n = 0;
  const store = createChatStore({
    api,
    realtime,
    userId: "me",
    getWorkspaceId: () => (opts.workspaceId === undefined ? "w1" : opts.workspaceId),
    newId: () => `cid-${++n}`,
    now: () => new Date("2026-02-02T00:00:00Z"),
  });
  realtime.connect();
  const sock = FakeSocket.last();
  sock.open();
  sock.receive("ready", { user_id: "me", device_id: "d1" });

  const emit = (type: string, data: object, s: FakeSocket = FakeSocket.last()) =>
    s.receive(type, { workspace_id: "w1", ...data });
  const created = (message: Message) => emit("message.created", { message });

  return { server, store, realtime, sock, emit, created };
}

const seqs = (store: ReturnType<typeof setup>["store"], channelId = "c1") =>
  store.getState().channels[channelId]!.messages.map((m) => m.seq);

let t: ReturnType<typeof setup>;

beforeEach(() => {
  t = setup();
  t.server.addChannel("c1");
});

describe("state", () => {
  it("loads channels with their unread counts", async () => {
    t.server.addChannel("c2", { last_seq: 5, last_read_seq: 2, unread_count: 3, mention_count: 1 });
    await t.store.loadChannels();
    const c2 = t.store.getState().channels.c2!;
    expect(c2).toMatchObject({ lastSeq: 5, lastReadSeq: 2, unreadCount: 3, mentionCount: 1, messages: [] });
  });

  it("replaces the snapshot on every change and notifies subscribers", async () => {
    const listener = vi.fn();
    t.store.subscribe(listener);
    const before = t.store.getState();
    await t.store.loadChannels();
    expect(t.store.getState()).not.toBe(before);
    expect(listener).toHaveBeenCalled();
    const after = t.store.getState();
    expect(t.store.getState()).toBe(after); // stable between changes
  });

  it("stops reacting after dispose", async () => {
    await t.store.loadChannels();
    t.store.dispose();
    t.created(t.server.add("c1", "bob", "hi"));
    await settle();
    expect(seqs(t.store)).toEqual([]);
  });
});

describe("ordering and duplicates", () => {
  beforeEach(async () => {
    await t.store.loadChannels();
  });

  it("appends in-order messages and counts them unread", async () => {
    t.created(t.server.add("c1", "bob", "one"));
    t.created(t.server.add("c1", "bob", "two"));
    await settle();
    const ch = t.store.getState().channels.c1!;
    expect(ch.messages.map((m) => m.body)).toEqual(["one", "two"]);
    expect(ch).toMatchObject({ lastSeq: 2, unreadCount: 2 });
    expect(t.server.requests.filter((r) => r.includes("/messages"))).toEqual([]);
  });

  it("sorts out-of-order delivery by seq and fetches the one it missed", async () => {
    const one = t.server.add("c1", "bob", "one");
    const two = t.server.add("c1", "bob", "two");
    t.created(two); // arrives first
    t.created(one);
    await settle();
    expect(seqs(t.store)).toEqual([1, 2]);
    expect(t.store.getState().channels.c1).toMatchObject({ lastSeq: 2, unreadCount: 2 });
  });

  it("keeps one copy when the same event is delivered twice", async () => {
    const m = t.server.add("c1", "bob", "hi");
    t.created(m);
    t.created(m);
    await settle();
    expect(seqs(t.store)).toEqual([1]);
    expect(t.store.getState().channels.c1!.unreadCount).toBe(1);
  });

  it("does not let a late duplicate undo an edit", async () => {
    const m = t.server.add("c1", "bob", "old");
    t.created(m);
    t.emit("message.updated", { message: { ...m, body: "new", edited_at: "2026-01-02T00:00:00Z" } });
    t.created(m); // stale replay of the original
    await settle();
    expect(t.store.getState().channels.c1!.messages[0]).toMatchObject({ body: "new" });
  });

  it("does not count own messages or deleted ones as unread", async () => {
    t.created(t.server.add("c1", "me", "mine"));
    await settle();
    expect(t.store.getState().channels.c1!.unreadCount).toBe(0);
  });

  it("counts mentions of me and of the channel", async () => {
    t.created(t.server.add("c1", "bob", "hey", { mentions: ["me"] }));
    t.created(t.server.add("c1", "bob", "all", { mentions_channel: true }));
    t.created(t.server.add("c1", "bob", "other", { mentions: ["zed"] }));
    await settle();
    expect(t.store.getState().channels.c1).toMatchObject({ unreadCount: 3, mentionCount: 2 });
  });
});

describe("gaps and catch-up", () => {
  beforeEach(async () => {
    await t.store.loadChannels();
  });

  it("fetches after_seq when a seq is skipped", async () => {
    t.created(t.server.add("c1", "bob", "1"));
    t.server.add("c1", "bob", "2"); // never delivered over the socket
    t.server.add("c1", "bob", "3");
    t.created(t.server.add("c1", "bob", "4"));
    await settle();
    expect(seqs(t.store)).toEqual([1, 2, 3, 4]);
    expect(t.server.requests).toContain("GET /channels/c1/messages?after_seq=1&limit=100");
    expect(t.store.getState().channels.c1).toMatchObject({ lastSeq: 4, unreadCount: 4 });
  });

  it("keeps fetching while has_more is true", async () => {
    t.server.pageSize = 2;
    for (let i = 1; i <= 5; i++) t.server.add("c1", "bob", String(i));
    t.created(t.server.messagesOf("c1")[4]!);
    await settle();
    expect(seqs(t.store)).toEqual([1, 2, 3, 4, 5]);
    const fetches = t.server.requests.filter((r) => r.includes("after_seq"));
    expect(fetches).toEqual([
      "GET /channels/c1/messages?after_seq=0&limit=100",
      "GET /channels/c1/messages?after_seq=2&limit=100",
      "GET /channels/c1/messages?after_seq=4&limit=100",
    ]);
  });

  it("catches up every known channel after a reconnect", async () => {
    t.server.addChannel("c2");
    await t.store.loadChannels();
    t.created(t.server.add("c1", "bob", "before"));
    await settle();

    t.sock.drop(); // network drop
    t.server.add("c1", "bob", "missed 1");
    t.server.add("c1", "bob", "missed 2");
    t.server.add("c2", "bob", "other channel");
    await vi.waitFor(() => expect(FakeSocket.instances).toHaveLength(2));
    const second = FakeSocket.last();
    second.open();
    second.receive("ready", { user_id: "me", device_id: "d1" });
    await settle();

    expect(t.store.getState().channels.c1!.messages.map((m) => m.body)).toEqual(["before", "missed 1", "missed 2"]);
    expect(t.store.getState().channels.c2!.messages.map((m) => m.body)).toEqual(["other channel"]);
    expect(t.server.requests).toContain("GET /channels/c1/messages?after_seq=1&limit=100");
    expect(t.server.requests).toContain("GET /channels/c2/messages?after_seq=0&limit=100");
  });

  it("runs one catch-up at a time per channel and repeats if asked again meanwhile", async () => {
    t.server.add("c1", "bob", "1");
    const first = t.store.catchUp("c1");
    const second = t.store.catchUp("c1");
    t.server.add("c1", "bob", "2");
    await Promise.all([first, second]);
    await settle();
    expect(seqs(t.store)).toEqual([1, 2]);
  });
});

describe("history", () => {
  it("loads the latest page, then older pages by before_seq", async () => {
    for (let i = 1; i <= 5; i++) t.server.add("c1", "bob", String(i));
    t.server.pageSize = 2;
    await t.store.loadChannels();
    await t.store.loadHistory("c1");
    expect(seqs(t.store)).toEqual([4, 5]);
    expect(t.store.getState().channels.c1!.hasMoreBefore).toBe(true);
    await t.store.loadHistory("c1");
    await t.store.loadHistory("c1");
    expect(seqs(t.store)).toEqual([1, 2, 3, 4, 5]);
    expect(t.store.getState().channels.c1!.hasMoreBefore).toBe(false);
    expect(t.server.requests.filter((r) => r.includes("/messages"))).toEqual([
      "GET /channels/c1/messages?limit=50",
      "GET /channels/c1/messages?before_seq=4&limit=50",
      "GET /channels/c1/messages?before_seq=2&limit=50",
    ]);
  });

  it("keeps the page order ascending whatever the page size", async () => {
    for (let i = 1; i <= 7; i++) t.server.add("c1", "bob", String(i));
    t.server.pageSize = 3;
    await t.store.loadChannels();
    await t.store.loadHistory("c1");
    expect(seqs(t.store)).toEqual([5, 6, 7]);
    await t.store.loadHistory("c1");
    expect(seqs(t.store)).toEqual([2, 3, 4, 5, 6, 7]);
  });

  it("catches up when the newest page is ahead of what the channel list said", async () => {
    t.server.add("c1", "bob", "1");
    await t.store.loadChannels();
    t.server.add("c1", "bob", "2");
    t.server.add("c1", "bob", "3");
    await t.store.loadHistory("c1");
    await settle();
    expect(seqs(t.store)).toEqual([1, 2, 3]);
    expect(t.store.getState().channels.c1!.lastSeq).toBe(3);
  });
});

describe("threads", () => {
  it("loads every reply of a thread without moving the channel history cursor", async () => {
    const root = t.server.add("c1", "bob", "root");
    for (let i = 0; i < 5; i++) t.server.add("c1", "bob", "filler");
    for (let i = 0; i < 3; i++) t.server.add("c1", "bob", `reply ${i}`, { thread_root_id: root.id });
    t.server.pageSize = 2;
    await t.store.loadChannels();
    await t.store.loadHistory("c1");
    expect(seqs(t.store)).toEqual([8, 9]);
    await t.store.loadThread("c1", root.id);
    const replies = t.store.getState().channels.c1!.messages.filter((m) => m.thread_root_id === root.id);
    expect(replies.map((m) => m.body)).toEqual(["reply 0", "reply 1", "reply 2"]);
    await t.store.loadHistory("c1");
    // The channel history continues below 8, where it stopped, not below the thread's oldest reply.
    expect(t.server.requests.filter((r) => r.includes("before_seq=") && !r.includes("thread_root_id")).at(-1)).toBe(
      "GET /channels/c1/messages?before_seq=8&limit=50",
    );
  });
});

describe("optimistic send", () => {
  beforeEach(async () => {
    await t.store.loadChannels();
  });

  const messages = () => t.store.getState().channels.c1!.messages;

  it("shows the message pending at once, with a client_msg_id", async () => {
    let release!: () => void;
    t.server.holdSendResponse = new Promise((r) => (release = r));
    const { clientMsgId, done } = t.store.send("c1", "hello");
    expect(clientMsgId).toBe("cid-1");
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ body: "hello", client_msg_id: "cid-1", sender_id: "me", status: "pending" });
    release();
    await done;
  });

  it("is confirmed by the POST response", async () => {
    await t.store.send("c1", "hello").done;
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ body: "hello", seq: 1, status: "sent", client_msg_id: "cid-1" });
    expect(messages()[0]!.id).toBe("m1");
    expect(t.store.getState().channels.c1).toMatchObject({ lastSeq: 1, unreadCount: 0 });
  });

  it("stays one message when the WebSocket echo arrives before the POST response", async () => {
    let release!: () => void;
    t.server.holdSendResponse = new Promise((r) => (release = r));
    const { done } = t.store.send("c1", "hello");
    await settle(); // the server stored it, the response is held
    t.created(t.server.messagesOf("c1")[0]!);
    await settle();
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ status: "sent", seq: 1 });
    release();
    await done;
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ status: "sent", seq: 1 });
  });

  it("stays one message when the echo arrives after the POST response", async () => {
    const { done } = t.store.send("c1", "hello");
    await done;
    t.created(t.server.messagesOf("c1")[0]!);
    await settle();
    expect(messages()).toHaveLength(1);
    expect(messages()[0]!.status).toBe("sent");
  });

  it("marks a failed send and keeps it in the list", async () => {
    t.server.failSends = 1;
    await t.store.send("c1", "hello").done;
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ status: "failed", client_msg_id: "cid-1" });
  });

  it("retries with the same client_msg_id and ends with one message", async () => {
    t.server.failSends = 1;
    await t.store.send("c1", "hello").done;
    await t.store.retry("c1", "cid-1");
    expect(t.server.posts.map((p) => p.client_msg_id)).toEqual(["cid-1", "cid-1"]);
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ status: "sent", seq: 1, body: "hello" });
    expect(t.server.messagesOf("c1")).toHaveLength(1);
  });

  it("retry shows the message pending again while it is in flight", async () => {
    t.server.failSends = 1;
    await t.store.send("c1", "hello").done;
    let release!: () => void;
    t.server.holdSendResponse = new Promise((r) => (release = r));
    const retrying = t.store.retry("c1", "cid-1");
    expect(messages()[0]!.status).toBe("pending");
    release();
    await retrying;
  });

  it("handles a send that the server stored although the response was lost", async () => {
    // First attempt reached the server but the client saw a failure.
    t.server.add("c1", "me", "hello", { client_msg_id: "cid-1" });
    t.server.failSends = 1;
    await t.store.send("c1", "hello").done;
    expect(messages()[0]!.status).toBe("failed");
    await t.store.retry("c1", "cid-1");
    expect(t.server.messagesOf("c1")).toHaveLength(1);
    expect(messages()).toHaveLength(1);
    expect(messages()[0]).toMatchObject({ status: "sent", seq: 1 });
  });

  it("keeps pending messages after confirmed ones, in send order", async () => {
    t.server.failSends = 2;
    await t.store.send("c1", "a").done;
    await t.store.send("c1", "b").done;
    t.created(t.server.add("c1", "bob", "from bob"));
    await settle();
    expect(messages().map((m) => m.body)).toEqual(["from bob", "a", "b"]);
  });

  it("drops a failed message on discard", async () => {
    t.server.failSends = 1;
    await t.store.send("c1", "hello").done;
    t.store.discard("c1", "cid-1");
    expect(messages()).toEqual([]);
  });

  it("refuses to send to a channel it does not know", () => {
    expect(() => t.store.send("nope", "x")).toThrow(/unknown channel/);
  });
});

describe("updates from the server", () => {
  beforeEach(async () => {
    await t.store.loadChannels();
    t.created(t.server.add("c1", "bob", "hello"));
    await settle();
  });

  const first = () => t.store.getState().channels.c1!.messages[0]!;

  it("applies message.updated", () => {
    const m = t.server.messagesOf("c1")[0]!;
    t.emit("message.updated", { message: { ...m, body: "hello!", edited_at: "2026-01-01T01:00:00Z" } });
    expect(first()).toMatchObject({ body: "hello!", edited_at: "2026-01-01T01:00:00Z" });
  });

  it("applies message.deleted: the row stays, the body is blanked", () => {
    const m = t.server.messagesOf("c1")[0]!;
    t.emit("message.deleted", {
      channel_id: "c1",
      message: { ...m, body: "", deleted_at: "2026-01-01T02:00:00Z" },
    });
    expect(first()).toMatchObject({ id: "m1", seq: 1, body: "", deleted_at: "2026-01-01T02:00:00Z" });
    expect(t.store.getState().channels.c1!.messages).toHaveLength(1);
  });

  it("applies card.updated to the payload", () => {
    t.emit("card.updated", { channel_id: "c1", message_id: "m1", payload: { status: "approved" } });
    expect(first().payload).toEqual({ status: "approved" });
  });

  it("applies channel.read from another device and clears the unread count", () => {
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 1, unread_count: 0, mention_count: 0 });
    expect(t.store.getState().channels.c1).toMatchObject({ lastReadSeq: 1, unreadCount: 0, mentionCount: 0 });
  });

  it("lowers the unread count for a partial read", async () => {
    t.created(t.server.add("c1", "bob", "two"));
    t.created(t.server.add("c1", "bob", "three"));
    await settle();
    expect(t.store.getState().channels.c1!.unreadCount).toBe(3);
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 2, unread_count: 1, mention_count: 0 });
    expect(t.store.getState().channels.c1).toMatchObject({ lastReadSeq: 2, unreadCount: 1 });
  });

  it("takes the counts of channel.read from the server, mentions included", async () => {
    t.created(t.server.add("c1", "bob", "two", { mentions_channel: true }));
    t.created(t.server.add("c1", "bob", "three"));
    await settle();
    expect(t.store.getState().channels.c1).toMatchObject({ unreadCount: 3, mentionCount: 1 });
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 2, unread_count: 1, mention_count: 0 });
    expect(t.store.getState().channels.c1).toMatchObject({ lastReadSeq: 2, unreadCount: 1, mentionCount: 0 });
  });

  it("never moves the read position back", () => {
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 1, unread_count: 0, mention_count: 0 });
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 0, unread_count: 5, mention_count: 2 });
    expect(t.store.getState().channels.c1).toMatchObject({ lastReadSeq: 1, unreadCount: 0, mentionCount: 0 });
  });

  it("catches up when channel.read is beyond what it has seen", async () => {
    t.server.add("c1", "bob", "two");
    t.server.add("c1", "bob", "three");
    t.emit("channel.read", { channel_id: "c1", last_read_seq: 3, unread_count: 0, mention_count: 0 });
    await settle();
    expect(seqs(t.store)).toEqual([1, 2, 3]);
  });

  it("applies the flat channel.updated and keeps the per-user counts it does not carry", async () => {
    t.created(t.server.add("c1", "bob", "two"));
    await settle();
    t.emit("channel.updated", {
      channel_id: "c1",
      type: "public",
      name: "renamed",
      topic: "new topic",
      last_seq: 2,
      member_count: 5,
      dm_user_ids: [],
      archived_at: null,
      created_at: "2026-01-01T00:00:00Z",
    });
    const ch = t.store.getState().channels.c1!;
    expect(ch.channel).toMatchObject({ id: "c1", name: "renamed", topic: "new topic", member_count: 5 });
    expect(ch).toMatchObject({ unreadCount: 2, lastReadSeq: 0 });
    expect(ch.messages).toHaveLength(2);
  });

  it("fetches a channel it has not seen when channel.updated names it", async () => {
    t.server.addChannel("c7", { name: "fresh" });
    t.emit("channel.updated", {
      channel_id: "c7", type: "public", name: "fresh", topic: "", last_seq: 0, member_count: 2,
      dm_user_ids: [], archived_at: null, created_at: "2026-01-01T00:00:00Z",
    });
    await settle();
    expect(t.store.getState().channels.c7!.channel.name).toBe("fresh");
  });

  it("catches up when channel.updated reports a newer last_seq", async () => {
    t.server.add("c1", "bob", "two");
    t.emit("channel.updated", {
      channel_id: "c1", type: "public", name: "c1", topic: "", last_seq: 2, member_count: 2,
      dm_user_ids: [], archived_at: null, created_at: "2026-01-01T00:00:00Z",
    });
    await settle();
    expect(seqs(t.store)).toEqual([1, 2]);
  });

  it("learns about a channel it has not seen from the first event", async () => {
    t.server.addChannel("c9");
    t.created(t.server.add("c9", "bob", "new dm"));
    await settle();
    const c9 = t.store.getState().channels.c9!;
    expect(c9.messages.map((m) => m.body)).toEqual(["new dm"]);
    expect(c9.lastSeq).toBe(1);
  });
});

describe("read state", () => {
  beforeEach(async () => {
    await t.store.loadChannels();
    t.created(t.server.add("c1", "bob", "one"));
    t.created(t.server.add("c1", "bob", "two"));
    await settle();
  });

  it("markRead posts the last seq and clears the counters", async () => {
    await t.store.markRead("c1");
    expect(t.server.requests).toContain("POST /channels/c1/read");
    expect(t.server.channels.get("c1")!.channel.last_read_seq).toBe(2);
    expect(t.store.getState().channels.c1).toMatchObject({ lastReadSeq: 2, unreadCount: 0, mentionCount: 0 });
  });

  it("does not post again when nothing is unread", async () => {
    await t.store.markRead("c1");
    await t.store.markRead("c1");
    expect(t.server.requests.filter((r) => r.startsWith("POST /channels/c1/read"))).toHaveLength(1);
  });

  it("does not count messages as unread while the channel is open, and marks them read", async () => {
    await t.store.setActiveChannel("c1");
    t.created(t.server.add("c1", "bob", "three"));
    await settle();
    expect(t.store.getState().channels.c1).toMatchObject({ lastSeq: 3, unreadCount: 0 });
    expect(t.server.channels.get("c1")!.channel.last_read_seq).toBe(3);
  });
});

describe("workspaces", () => {
  it("drops events of another workspace but counts them as unread for it", async () => {
    await t.store.loadChannels();
    FakeSocket.last().receive("message.created", {
      workspace_id: "w2",
      message: { ...t.server.add("c1", "bob", "elsewhere") },
    });
    FakeSocket.last().receive("message.created", {
      workspace_id: "w2",
      message: { ...t.server.add("c1", "me", "mine") },
    });
    await settle();
    expect(seqs(t.store)).toEqual([]);
    expect(t.store.getState().workspaceUnread).toEqual({ w2: 1 });
  });

  it("reset forgets everything", async () => {
    await t.store.loadChannels();
    t.store.reset();
    expect(t.store.getState().channels).toEqual({});
  });

  it("ignores a channel list that arrives after reset", async () => {
    const pending = t.store.loadChannels();
    t.store.reset();
    await pending;
    expect(t.store.getState().channels).toEqual({});
  });
});
