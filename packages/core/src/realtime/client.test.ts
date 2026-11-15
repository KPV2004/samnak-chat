import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createRealtimeClient, type RealtimeClient, type RealtimeOptions } from "./client";
import { FakeSocket } from "./fake-socket";

let token: string | null = "tok1";

function setup(extra: Partial<RealtimeOptions> = {}): RealtimeClient {
  return createRealtimeClient({
    url: "ws://x/api/v1/ws",
    createSocket: (url) => new FakeSocket(url),
    getToken: () => token,
    random: () => 1, // no jitter: delays are exactly the backoff steps
    ...extra,
  });
}

/** Connects and completes the handshake on the newest socket. */
function ready(userId = "u1") {
  const s = FakeSocket.last();
  s.open();
  s.receive("ready", { user_id: userId, device_id: "d1" });
  return s;
}

beforeEach(() => {
  vi.useFakeTimers();
  FakeSocket.reset();
  token = "tok1";
});
afterEach(() => {
  vi.useRealTimers();
});

describe("handshake", () => {
  it("sends the current access token as the first frame", () => {
    const rt = setup();
    rt.connect();
    const s = FakeSocket.last();
    expect(s.url).toBe("ws://x/api/v1/ws");
    expect(s.sent).toEqual([]);
    s.open();
    expect(s.sent).toEqual([{ type: "auth", data: { access_token: "tok1" } }]);
    expect(rt.getStatus()).toBe("authenticating");
  });

  it("is ready after the server's ready frame", () => {
    const rt = setup();
    const onReady = vi.fn();
    rt.on("ready", onReady);
    rt.connect();
    ready("u9");
    expect(rt.getStatus()).toBe("ready");
    expect(onReady).toHaveBeenCalledWith({ user_id: "u9", device_id: "d1" });
  });

  it("does not connect without a token", () => {
    token = null;
    const rt = setup();
    rt.connect();
    expect(FakeSocket.instances).toHaveLength(0);
    expect(rt.getStatus()).toBe("closed");
  });

  it("gives up on a handshake that never completes and reconnects", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().open();
    vi.advanceTimersByTime(10_000);
    expect(FakeSocket.instances[0]!.closed).not.toBeNull();
    vi.advanceTimersByTime(1_000);
    expect(FakeSocket.instances).toHaveLength(2);
  });

  it("notifies subscribers of status changes", () => {
    const rt = setup();
    const listener = vi.fn();
    rt.subscribe(listener);
    rt.connect();
    expect(listener).toHaveBeenCalledTimes(1);
    ready();
    expect(listener.mock.calls.length).toBeGreaterThanOrEqual(3);
  });
});

describe("events", () => {
  it("delivers typed events to handlers until unsubscribed", () => {
    const rt = setup();
    const handler = vi.fn();
    const off = rt.on("channel.read", handler);
    rt.connect();
    const s = ready();
    const data = { workspace_id: "w1", channel_id: "c1", last_read_seq: 4, unread_count: 0, mention_count: 0 };
    s.receive("channel.read", data);
    expect(handler).toHaveBeenCalledWith(data);
    off();
    s.receive("channel.read", { ...data, last_read_seq: 5 });
    expect(handler).toHaveBeenCalledTimes(1);
  });

  it("ignores malformed frames, unknown types and pong", () => {
    const rt = setup();
    const handler = vi.fn();
    rt.on("typing", handler);
    rt.connect();
    const s = ready();
    s.receiveRaw("not json");
    s.receiveRaw(JSON.stringify([1, 2]));
    s.receiveRaw(42);
    s.receive("pong");
    s.receive("something.new", {});
    expect(handler).not.toHaveBeenCalled();
    expect(rt.getStatus()).toBe("ready");
  });

  it("delivers an error frame and keeps the connection open", () => {
    const rt = setup();
    const handler = vi.fn();
    rt.on("error", handler);
    rt.connect();
    const s = ready();
    s.receive("error", { type: "typing", code: "invalid", message: "bad channel_id" });
    expect(handler).toHaveBeenCalledWith({ type: "typing", code: "invalid", message: "bad channel_id" });
    expect(s.closed).toBeNull();
    expect(rt.getStatus()).toBe("ready");
  });

  it("delivers presence.changed, which carries no workspace_id", () => {
    const rt = setup();
    const handler = vi.fn();
    rt.on("presence.changed", handler);
    rt.connect();
    ready().receive("presence.changed", { user_id: "u2", online: true });
    expect(handler).toHaveBeenCalledWith({ user_id: "u2", online: true });
  });

  it("keeps delivering when one handler throws", () => {
    const rt = setup();
    const second = vi.fn();
    rt.on("typing", () => {
      throw new Error("boom");
    });
    rt.on("typing", second);
    rt.connect();
    ready().receive("typing", { workspace_id: "w1" });
    expect(second).toHaveBeenCalled();
  });
});

describe("ping", () => {
  it("pings every 30 seconds once ready, and not before", () => {
    const rt = setup();
    rt.connect();
    const s = FakeSocket.last();
    s.open();
    vi.advanceTimersByTime(9_000); // still inside the handshake window
    expect(s.sent.filter((f) => f.type === "ping")).toHaveLength(0);
    s.receive("ready", { user_id: "u1", device_id: "d1" });
    vi.advanceTimersByTime(30_000);
    expect(s.sent.filter((f) => f.type === "ping")).toHaveLength(1);
    s.receive("pong");
    vi.advanceTimersByTime(30_000);
    expect(s.sent.filter((f) => f.type === "ping")).toHaveLength(2);
  });

  it("treats a missing pong as a dropped connection and reconnects", () => {
    const rt = setup();
    rt.connect();
    const s = ready();
    vi.advanceTimersByTime(30_000); // ping 1, never answered
    vi.advanceTimersByTime(30_000); // deadline passed
    expect(s.closed).not.toBeNull();
    vi.advanceTimersByTime(1_000);
    expect(FakeSocket.instances).toHaveLength(2);
  });
});

describe("reconnect", () => {
  it("backs off exponentially from 1 s to 30 s", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().drop();
    const delays: number[] = [];
    for (let i = 0; i < 7; i++) {
      const before = FakeSocket.instances.length;
      let waited = 0;
      while (FakeSocket.instances.length === before) {
        vi.advanceTimersByTime(500);
        waited += 500;
      }
      delays.push(waited);
      FakeSocket.last().drop(); // the attempt fails before the handshake
    }
    expect(delays).toEqual([1000, 2000, 4000, 8000, 16000, 30000, 30000]);
    expect(rt.getStatus()).toBe("reconnecting");
  });

  it("applies jitter between half the step and the full step", () => {
    const rt = setup({ random: () => 0 });
    rt.connect();
    FakeSocket.last().drop();
    vi.advanceTimersByTime(499);
    expect(FakeSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1);
    expect(FakeSocket.instances).toHaveLength(2);
  });

  it("resets the backoff after a successful handshake", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().drop();
    vi.advanceTimersByTime(1_000);
    FakeSocket.last().drop();
    vi.advanceTimersByTime(2_000);
    ready();
    FakeSocket.last().drop();
    vi.advanceTimersByTime(1_000);
    expect(FakeSocket.instances).toHaveLength(4);
  });

  it("emits ready again after reconnecting so listeners can catch up", () => {
    const rt = setup();
    const onReady = vi.fn();
    rt.on("ready", onReady);
    rt.connect();
    ready().drop();
    vi.advanceTimersByTime(1_000);
    ready();
    expect(onReady).toHaveBeenCalledTimes(2);
  });

  it("uses the token current at each attempt", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().drop();
    token = "tok2";
    vi.advanceTimersByTime(1_000);
    FakeSocket.last().open();
    expect(FakeSocket.last().sent[0]).toEqual({ type: "auth", data: { access_token: "tok2" } });
    expect(rt).toBeDefined();
  });

  it("does not reconnect after close()", () => {
    const rt = setup();
    rt.connect();
    const s = ready();
    rt.close();
    expect(s.closed).toEqual({ code: 1000 });
    expect(rt.getStatus()).toBe("closed");
    vi.advanceTimersByTime(60_000);
    expect(FakeSocket.instances).toHaveLength(1);
  });

  it("cancels a pending reconnect on close()", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().drop();
    rt.close();
    vi.advanceTimersByTime(60_000);
    expect(FakeSocket.instances).toHaveLength(1);
  });
});

describe("close codes", () => {
  it.each([
    [1011, "server error"],
    [1008, "idle timeout"],
    [1001, "server shutdown"],
    [1006, "dropped connection"],
  ])("reconnects with backoff after %i (%s)", (code) => {
    const rt = setup();
    rt.connect();
    ready();
    FakeSocket.last().drop(code);
    expect(rt.getStatus()).toBe("reconnecting");
    vi.advanceTimersByTime(1_000);
    expect(FakeSocket.instances).toHaveLength(2);
  });

  it("without a refresh handler, 4401 retries with backoff instead of looping", () => {
    const rt = setup();
    rt.connect();
    FakeSocket.last().open();
    FakeSocket.last().drop(4401);
    expect(FakeSocket.instances).toHaveLength(1);
    vi.advanceTimersByTime(1_000);
    expect(FakeSocket.instances).toHaveLength(2);
  });
});

describe("re-auth", () => {
  it("refreshes the token on close 4401 and reconnects at once with the new one", async () => {
    const onUnauthorized = vi.fn(async (rejected: string) => {
      expect(rejected).toBe("tok1");
      token = "tok2";
      return true;
    });
    const rt = setup({ onUnauthorized });
    rt.connect();
    FakeSocket.last().open();
    FakeSocket.last().drop(4401);
    await vi.advanceTimersByTimeAsync(0);
    expect(FakeSocket.instances).toHaveLength(2);
    FakeSocket.last().open();
    expect(FakeSocket.last().sent[0]).toEqual({ type: "auth", data: { access_token: "tok2" } });
  });

  it("stops when the token cannot be refreshed", async () => {
    const rt = setup({ onUnauthorized: async () => false });
    rt.connect();
    FakeSocket.last().open();
    FakeSocket.last().drop(4401);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(FakeSocket.instances).toHaveLength(1);
    expect(rt.getStatus()).toBe("closed");
  });

  it("backs off instead of looping when the refreshed token is still rejected", async () => {
    const rt = setup({ onUnauthorized: async () => true });
    rt.connect();
    FakeSocket.last().open();
    FakeSocket.last().drop(4401);
    await vi.advanceTimersByTimeAsync(0);
    expect(FakeSocket.instances).toHaveLength(1);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(FakeSocket.instances).toHaveLength(2);
  });

  it("reauth() reconnects immediately so the new access token is used", () => {
    const rt = setup();
    rt.connect();
    const first = ready();
    token = "tok2";
    rt.reauth();
    expect(first.closed).not.toBeNull();
    expect(FakeSocket.instances).toHaveLength(2);
    FakeSocket.last().open();
    expect(FakeSocket.last().sent[0]).toEqual({ type: "auth", data: { access_token: "tok2" } });
  });

  it("reauth() does nothing before connect()", () => {
    const rt = setup();
    rt.reauth();
    expect(FakeSocket.instances).toHaveLength(0);
  });
});

describe("send", () => {
  it("sends envelopes only while ready", () => {
    const rt = setup();
    rt.connect();
    const s = FakeSocket.last();
    s.open();
    expect(rt.send("typing", { channel_id: "c1" })).toBe(false);
    s.receive("ready", { user_id: "u1", device_id: "d1" });
    expect(rt.send("typing", { channel_id: "c1" })).toBe(true);
    expect(s.sent.at(-1)).toEqual({ type: "typing", data: { channel_id: "c1" } });
  });
});
