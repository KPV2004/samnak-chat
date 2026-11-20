import type { FetchInit, FetchLike, FetchResponse } from "../api/client";
import type { Channel, Message } from "../api/m2";

interface FakeChannel {
  channel: Channel;
  messages: Message[];
}

/**
 * An in-memory stand-in for the chat endpoints, behind a fake `fetch`.
 * It assigns `seq` and dedupes by `(sender_id, client_msg_id)` like the real one.
 */
export class FakeChatServer {
  channels = new Map<string, FakeChannel>();
  /** Every request, in order, as `METHOD path?query`. */
  requests: string[] = [];
  /** Bodies of message POSTs, in order. */
  posts: { client_msg_id: string; body: string }[] = [];
  /** The next this-many message POSTs fail with a network error. */
  failSends = 0;
  /** While set, message POSTs store the message but wait for this before answering. */
  holdSendResponse: Promise<void> | null = null;
  pageSize = 100;
  private nextId = 1;

  addChannel(id: string, patch: Partial<Channel> = {}): Channel {
    const channel: Channel = {
      id,
      type: "public",
      name: id,
      topic: "",
      last_seq: 0,
      last_read_seq: 0,
      unread_count: 0,
      mention_count: 0,
      member_count: 2,
      dm_user_ids: [],
      archived_at: null,
      pinned_at: null,
      last_message: null,
      created_at: "2026-01-01T00:00:00Z",
      ...patch,
    };
    this.channels.set(id, { channel, messages: [] });
    return channel;
  }

  /** Stores a message as the server would after a send and returns it. */
  add(channelId: string, sender: string, body: string, patch: Partial<Message> = {}): Message {
    const ch = this.must(channelId);
    const seq = ++ch.channel.last_seq;
    const message: Message = {
      id: `m${this.nextId++}`,
      channel_id: channelId,
      seq,
      sender_id: sender,
      client_msg_id: `k${seq}-${sender}`,
      kind: "text",
      body,
      payload: null,
      fallback_text: "",
      thread_root_id: null,
      reply_count: 0,
      attachments: [],
      reactions: [],
      mentions: [],
      mentions_channel: false,
      edited_at: null,
      deleted_at: null,
      created_at: `2026-01-01T00:00:${String(seq % 60).padStart(2, "0")}Z`,
      ...patch,
    };
    ch.messages.push(message);
    return message;
  }

  messagesOf(channelId: string): Message[] {
    return this.must(channelId).messages;
  }

  fetch: FetchLike = async (url, init = {}) => {
    const [rawPath = "", search = ""] = url.replace(/^https?:\/\/[^/]*/, "").split("?");
    const path = rawPath.replace("/api/v1/chat", "");
    this.requests.push(`${init.method ?? "GET"} ${path}${search ? `?${search}` : ""}`);
    const query = new Map(search.split("&").filter(Boolean).map((kv) => kv.split("=") as [string, string]));
    return this.route(init, path, query);
  };

  private must(id: string): FakeChannel {
    const ch = this.channels.get(id);
    if (!ch) throw new Error(`fake server: no channel ${id}`);
    return ch;
  }

  private async route(init: FetchInit, path: string, q: Map<string, string>): Promise<FetchResponse> {
    const method = init.method ?? "GET";
    const body = init.body ? JSON.parse(init.body) : undefined;
    let m: RegExpMatchArray | null;

    if (method === "GET" && path === "/channels") return ok({ items: [...this.channels.values()].map((c) => c.channel) });
    if ((m = path.match(/^\/channels\/([^/]+)$/)) && method === "GET") {
      const ch = this.channels.get(decodeURIComponent(m[1]!));
      return ch ? ok(ch.channel) : fail(404);
    }
    if ((m = path.match(/^\/channels\/([^/]+)\/messages$/))) {
      const ch = this.channels.get(m[1]!);
      if (!ch) return fail(404);
      if (method === "GET") return ok(this.page(ch, q));
      return this.send(ch, body);
    }
    if ((m = path.match(/^\/channels\/([^/]+)\/read$/)) && method === "POST") {
      const ch = this.must(m[1]!);
      ch.channel.last_read_seq = Math.max(ch.channel.last_read_seq, body.seq);
      return { ok: true, status: 204, statusText: "", json: async () => undefined };
    }
    throw new Error(`fake server: unhandled ${method} ${path}`);
  }

  private page(ch: FakeChannel, q: Map<string, string>) {
    const limit = Math.min(Number(q.get("limit") ?? 50), this.pageSize);
    if (q.has("after_seq")) {
      const after = Number(q.get("after_seq"));
      const rest = ch.messages.filter((x) => x.seq > after);
      return { items: rest.slice(0, limit), has_more: rest.length > limit };
    }
    // No cursor, or 0, means the newest page; pages are ascending like the server's.
    const before = Number(q.get("before_seq") ?? 0) || Infinity;
    const thread = q.get("thread_root_id");
    const older = ch.messages.filter((x) => x.seq < before && (!thread || x.thread_root_id === thread));
    return { items: older.slice(-limit), has_more: older.length > limit };
  }

  private async send(ch: FakeChannel, body: { client_msg_id: string; body: string; sender?: string }): Promise<FetchResponse> {
    this.posts.push({ client_msg_id: body.client_msg_id, body: body.body });
    if (this.failSends > 0) {
      this.failSends--;
      throw new TypeError("network down");
    }
    const existing = ch.messages.find((x) => x.client_msg_id === body.client_msg_id);
    const stored = existing ?? this.add(ch.channel.id, this.me, body.body, { client_msg_id: body.client_msg_id });
    if (this.holdSendResponse) await this.holdSendResponse;
    return ok(stored, existing ? 200 : 201);
  }

  /** The signed-in user, who owns messages sent through `POST`. */
  me = "me";
}

function ok(body: unknown, status = 200): FetchResponse {
  return { ok: true, status, statusText: "", json: async () => JSON.parse(JSON.stringify(body)) };
}

function fail(status: number): FetchResponse {
  return { ok: false, status, statusText: "", json: async () => ({ error: { code: "not_found", message: "not found" } }) };
}
