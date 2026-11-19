import { describe, expect, it } from "vitest";
import { ApiError, createApiClient, type FetchInit, type FetchResponse } from "./client";
import { createM3Api, type NotificationItem, type NotifyPreferences } from "./m3";

const mention: NotificationItem = {
  id: "n1",
  kind: "chat.mention",
  title: "Somchai in #general",
  body: "hi @Mali",
  link: "/chat/c1",
  data: { channel_id: "c1", message_id: "m1", seq: 3 },
  read_at: null,
  created_at: "2026-01-01T09:00:00Z",
};

const ok = (body: unknown): FetchResponse => ({
  ok: true,
  status: 200,
  statusText: "",
  json: async () => JSON.parse(JSON.stringify(body)),
});

const noContent = (): FetchResponse => ({ ok: true, status: 204, statusText: "", json: async () => null });

function setup() {
  const requests: string[] = [];
  const fetch = async (url: string, init: FetchInit = {}): Promise<FetchResponse> => {
    requests.push(`${init.method ?? "GET"} ${url}`);
    const path = url.split("?")[0]!;
    if (path === "/api/v1/notify/unread-count") return ok({ count: 7 });
    if (path === "/api/v1/notify/notifications") return ok({ items: [mention], has_more: true });
    if (path === "/api/v1/notify/read-all") return noContent();
    if (/^\/api\/v1\/notify\/notifications\/missing\/read$/.test(path)) {
      return {
        ok: false,
        status: 404,
        statusText: "not found",
        json: async () => ({ error: { code: "not_found", message: "no such notification" } }),
      };
    }
    if (/^\/api\/v1\/notify\/notifications\/[^/]+\/read$/.test(path)) return noContent();
    return {
      ok: false,
      status: 404,
      statusText: "not found",
      json: async () => ({ error: { code: "not_found", message: "no such endpoint" } }),
    };
  };
  return { api: createM3Api(createApiClient({ baseUrl: "", fetch })), requests };
}

describe("notify api", () => {
  it("lists notifications with the query the plan defines", async () => {
    const { api, requests } = setup();
    await expect(api.listNotifications()).resolves.toMatchObject({ items: [mention], has_more: true });
    await api.listNotifications({ unread: true, before: "n9", limit: 5 });
    expect(requests).toEqual([
      "GET /api/v1/notify/notifications",
      "GET /api/v1/notify/notifications?unread=true&before=n9&limit=5",
    ]);
  });

  it("reads the unread count", async () => {
    const { api, requests } = setup();
    await expect(api.unreadNotificationCount()).resolves.toEqual({ count: 7 });
    expect(requests).toEqual(["GET /api/v1/notify/unread-count"]);
  });

  it("marks one notification read, and encodes the id", async () => {
    const { api, requests } = setup();
    await expect(api.readNotification("n/1")).resolves.toBeUndefined();
    expect(requests).toEqual(["POST /api/v1/notify/notifications/n%2F1/read"]);
  });

  it("marks every notification read", async () => {
    const { api, requests } = setup();
    await expect(api.readAllNotifications()).resolves.toBeUndefined();
    expect(requests).toEqual(["POST /api/v1/notify/read-all"]);
  });

  it("surfaces the server's error body", async () => {
    const { api } = setup();
    const error = await api.readNotification("missing").catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ApiError);
    expect(error).toMatchObject({ status: 404, code: "not_found", message: "no such notification" });
  });
});

describe("notify preferences api", () => {
  const prefs: NotifyPreferences = {
    disabled_kinds: ["chat.dm"],
    muted_channel_ids: ["c1"],
    dnd_start: "22:00",
    dnd_end: "07:00",
  };

  function setup() {
    const requests: { method: string; url: string; body?: string; headers?: Record<string, string> }[] = [];
    const fetch = async (url: string, init: FetchInit = {}): Promise<FetchResponse> => {
      requests.push({ method: init.method ?? "GET", url, body: init.body, headers: init.headers });
      return ok(prefs);
    };
    return { api: createM3Api(createApiClient({ baseUrl: "", fetch })), requests };
  }

  it("reads my settings", async () => {
    const { api, requests } = setup();
    await expect(api.getPreferences()).resolves.toEqual(prefs);
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ method: "GET", url: "/api/v1/notify/preferences" });
  });

  it("replaces my settings with the whole object", async () => {
    const { api, requests } = setup();
    await expect(api.setPreferences(prefs)).resolves.toEqual(prefs);
    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ method: "PUT", url: "/api/v1/notify/preferences" });
    expect(requests[0]!.headers).toMatchObject({ "Content-Type": "application/json" });
    expect(JSON.parse(requests[0]!.body!)).toEqual(prefs);
  });
});
