import type { FetchInit, FetchLike, FetchResponse } from "../api/client";
import type { NotificationItem } from "../api/m3";

/**
 * An in-memory stand-in for the notify endpoints, behind a fake `fetch`.
 * `items` is newest first, the order the API returns, so `add` prepends.
 */
export class FakeNotifyServer {
  items: NotificationItem[] = [];
  /** Every request, in order, as `METHOD path?query`. */
  requests: string[] = [];
  /** The next this-many unread-count requests fail. */
  failCounts = 0;
  /** The next this-many list requests fail. */
  failLists = 0;
  /** The next this-many read calls fail. */
  failReads = 0;
  private nextId = 1;

  /** Stores a notification and returns it; the newest one is first. */
  add(patch: Partial<NotificationItem> = {}): NotificationItem {
    const n = this.nextId++;
    const notification: NotificationItem = {
      id: `n${n}`,
      kind: "system.notice",
      title: `Title ${n}`,
      body: `Body ${n}`,
      link: `/chat/c1`,
      data: {},
      read_at: null,
      created_at: `2026-01-01T00:00:${String(n).padStart(2, "0")}Z`,
      ...patch,
    };
    this.items.unshift(notification);
    return notification;
  }

  unread(): number {
    return this.items.filter((n) => !n.read_at).length;
  }

  fetch: FetchLike = async (url, init = {}) => {
    const [rawPath = "", search = ""] = url.replace(/^https?:\/\/[^/]*/, "").split("?");
    const path = rawPath.replace("/api/v1/notify", "");
    this.requests.push(`${init.method ?? "GET"} ${path}${search ? `?${search}` : ""}`);
    const query = new Map(search.split("&").filter(Boolean).map((kv) => kv.split("=") as [string, string]));
    return this.route(init, path, query);
  };

  private async route(init: FetchInit, path: string, q: Map<string, string>): Promise<FetchResponse> {
    const method = init.method ?? "GET";
    if (method === "GET" && path === "/notifications") {
      if (this.failLists > 0) {
        this.failLists--;
        return fail(500);
      }
      return ok(this.page(q));
    }
    if (method === "GET" && path === "/unread-count") {
      if (this.failCounts > 0) {
        this.failCounts--;
        return fail(500);
      }
      return ok({ count: this.unread() });
    }
    if (method === "POST" && path === "/read-all") {
      if (this.failReads > 0) {
        this.failReads--;
        return fail(500);
      }
      const readAt = "2026-02-02T00:00:00Z";
      this.items = this.items.map((n) => (n.read_at ? n : { ...n, read_at: readAt }));
      return noContent();
    }
    const m = path.match(/^\/notifications\/([^/]+)\/read$/);
    if (method === "POST" && m) {
      const id = decodeURIComponent(m[1]!);
      const found = this.items.find((n) => n.id === id);
      if (!found) return fail(404);
      if (this.failReads > 0) {
        this.failReads--;
        return fail(500);
      }
      this.items = this.items.map((n) => (n.id === id ? { ...n, read_at: n.read_at ?? "2026-02-02T00:00:00Z" } : n));
      return noContent();
    }
    throw new Error(`fake notify server: unhandled ${method} ${path}`);
  }

  private page(q: Map<string, string>) {
    const limit = Math.min(Number(q.get("limit") ?? 20), 50);
    let list = this.items;
    if (q.get("unread") === "true") list = list.filter((n) => !n.read_at);
    const before = q.get("before");
    if (before) {
      const at = list.findIndex((n) => n.id === before);
      list = at < 0 ? [] : list.slice(at + 1);
    }
    return { items: list.slice(0, limit), has_more: list.length > limit };
  }
}

function ok(body: unknown): FetchResponse {
  return { ok: true, status: 200, statusText: "", json: async () => JSON.parse(JSON.stringify(body)) };
}

function noContent(): FetchResponse {
  return { ok: true, status: 204, statusText: "", json: async () => null };
}

function fail(status: number): FetchResponse {
  return {
    ok: false,
    status,
    statusText: "failed",
    json: async () => ({ error: { code: "internal", message: "failed" } }),
  };
}
