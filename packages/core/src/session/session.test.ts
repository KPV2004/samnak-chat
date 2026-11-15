import { describe, expect, it, vi } from "vitest";
import type { FetchInit } from "../api/client";
import { createSession, MemoryTokenStore, type SessionData } from "./session";

function res(body: unknown, status = 200) {
  return { ok: status < 400, status, statusText: "", json: async () => body };
}

const user = {
  id: "u1",
  email: "a@b.co",
  display_name: "A",
  avatar_url: "",
  locale: "th",
  timezone: "Asia/Bangkok",
  created_at: "2026-01-01T00:00:00Z",
};

function pair(n: number) {
  return { access_token: `acc${n}`, refresh_token: `ref${n}`, expires_in: 900, device_id: "d1", user };
}

function signedIn(extra: Partial<SessionData> = {}): MemoryTokenStore {
  return new MemoryTokenStore({ accessToken: "acc0", refreshToken: "ref0", deviceId: "d1", workspaceId: null, ...extra });
}

function auth(init?: FetchInit) {
  return init?.headers?.Authorization;
}

describe("session requests", () => {
  it("sends Authorization and X-Workspace-ID", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res({ items: [] }));
    const session = createSession({ baseUrl: "", store: signedIn({ workspaceId: "w1" }), fetch: fetchMock });
    await session.client.request("/api/v1/workspace/members");
    const headers = fetchMock.mock.calls[0]![1].headers;
    expect(headers.Authorization).toBe("Bearer acc0");
    expect(headers["X-Workspace-ID"]).toBe("w1");
  });

  it("omits X-Workspace-ID when no workspace is selected", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res({}));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await session.client.request("/api/v1/auth/me");
    expect(fetchMock.mock.calls[0]![1].headers["X-Workspace-ID"]).toBeUndefined();
  });

  it("returns undefined for 204 responses", async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 204,
      statusText: "",
      json: async () => {
        throw new Error("no body");
      },
    });
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await expect(session.client.request("/api/v1/auth/devices/x", { method: "DELETE" })).resolves.toBeUndefined();
  });
});

describe("session refresh", () => {
  it("refreshes once on 401, retries with the new token and stores the rotated pair", async () => {
    const calls: Array<{ url: string; init?: FetchInit }> = [];
    const fetchMock = vi.fn(async (url: string, init?: FetchInit) => {
      calls.push({ url, init });
      if (url.endsWith("/auth/refresh")) return res(pair(1));
      return auth(init) === "Bearer acc1" ? res(user) : res({ error: { code: "unauthorized", message: "no" } }, 401);
    });
    const store = signedIn();
    const session = createSession({ baseUrl: "", store, fetch: fetchMock });

    await expect(session.client.request("/api/v1/auth/me")).resolves.toEqual(user);

    expect(calls.map((c) => c.url)).toEqual(["/api/v1/auth/me", "/api/v1/auth/refresh", "/api/v1/auth/me"]);
    expect(JSON.parse(calls[1]!.init!.body!)).toEqual({ refresh_token: "ref0" });
    expect(store.load()).toMatchObject({ accessToken: "acc1", refreshToken: "ref1" });
  });

  it("shares a single in-flight refresh between concurrent requests", async () => {
    let refreshes = 0;
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const fetchMock = vi.fn(async (url: string, init?: FetchInit) => {
      if (url.endsWith("/auth/refresh")) {
        refreshes++;
        await gate;
        return res(pair(1));
      }
      return auth(init) === "Bearer acc1" ? res({ ok: url }) : res({ error: { code: "unauthorized", message: "no" } }, 401);
    });
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });

    const all = Promise.all([
      session.client.request("/api/v1/a"),
      session.client.request("/api/v1/b"),
      session.client.request("/api/v1/c"),
    ]);
    await vi.waitFor(() => expect(refreshes).toBe(1));
    release();
    await expect(all).resolves.toEqual([{ ok: "/api/v1/a" }, { ok: "/api/v1/b" }, { ok: "/api/v1/c" }]);
    expect(refreshes).toBe(1);
  });

  it("does not refresh again when a late 401 used a token that was already replaced", async () => {
    let refreshes = 0;
    let slowStarted!: () => void;
    const started = new Promise<void>((r) => (slowStarted = r));
    let releaseSlow!: () => void;
    const slowGate = new Promise<void>((r) => (releaseSlow = r));
    const fetchMock = vi.fn(async (url: string, init?: FetchInit) => {
      if (url.endsWith("/auth/refresh")) {
        refreshes++;
        return res(pair(1));
      }
      if (url.endsWith("/slow") && auth(init) === "Bearer acc0") {
        slowStarted();
        await slowGate;
        return res({ error: { code: "unauthorized", message: "no" } }, 401);
      }
      return auth(init) === "Bearer acc1" ? res({ ok: url }) : res({ error: { code: "unauthorized", message: "no" } }, 401);
    });
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });

    const slow = session.client.request("/api/v1/slow");
    await started;
    await session.client.request("/api/v1/fast"); // 401 -> refresh -> acc1
    releaseSlow();
    await expect(slow).resolves.toEqual({ ok: "/api/v1/slow" });
    expect(refreshes).toBe(1);
  });

  it("does not retry more than once when the retried request is still 401", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.endsWith("/auth/refresh")) return res(pair(1));
      return res({ error: { code: "unauthorized", message: "no" } }, 401);
    });
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await expect(session.client.request("/api/v1/x")).rejects.toMatchObject({ status: 401 });
    expect(fetchMock.mock.calls.map((c) => c[0])).toEqual(["/api/v1/x", "/api/v1/auth/refresh", "/api/v1/x"]);
  });

  it("signs out when the refresh fails and rejects the original request", async () => {
    const fetchMock = vi.fn(async (url: string) =>
      url.endsWith("/auth/refresh")
        ? res({ error: { code: "unauthorized", message: "revoked" } }, 401)
        : res({ error: { code: "unauthorized", message: "no" } }, 401),
    );
    const store = signedIn({ workspaceId: "w1" });
    const session = createSession({ baseUrl: "", store, fetch: fetchMock });
    const seen: boolean[] = [];
    session.subscribe(() => seen.push(session.getState().signedIn));

    await expect(session.client.request("/api/v1/auth/me")).rejects.toMatchObject({ status: 401 });

    expect(store.load()).toBeNull();
    expect(session.getState()).toEqual({ signedIn: false, deviceId: null, workspaceId: null });
    expect(seen).toEqual([false]);
  });

  it("stays signed in when the refresh fails because the network is down", async () => {
    const fetchMock = vi.fn(async (url: string) => {
      if (url.endsWith("/auth/refresh")) throw new TypeError("offline");
      return res({ error: { code: "unauthorized", message: "no" } }, 401);
    });
    const store = signedIn();
    const session = createSession({ baseUrl: "", store, fetch: fetchMock });
    await expect(session.client.request("/api/v1/x")).rejects.toBeInstanceOf(TypeError);
    expect(store.load()).not.toBeNull();
  });

  it("does not try to refresh when signed out", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res({ error: { code: "unauthorized", message: "no" } }, 401));
    const session = createSession({ baseUrl: "", store: new MemoryTokenStore(), fetch: fetchMock });
    await expect(session.client.request("/api/v1/x")).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

describe("session sign in and out", () => {
  it("login stores the tokens and keeps no stale workspace", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res(pair(1)));
    const store = new MemoryTokenStore();
    const session = createSession({ baseUrl: "", store, fetch: fetchMock });
    const seen: boolean[] = [];
    session.subscribe(() => seen.push(session.getState().signedIn));

    const tokens = await session.login({ email: "a@b.co", password: "pw", device_name: "Chrome", platform: "web" });

    expect(tokens.user).toEqual(user);
    expect(session.getState()).toEqual({ signedIn: true, deviceId: "d1", workspaceId: null });
    expect(store.load()).toEqual({ accessToken: "acc1", refreshToken: "ref1", deviceId: "d1", workspaceId: null });
    expect(seen).toEqual([true]);
    expect(fetchMock.mock.calls[0]![1].headers.Authorization).toBeUndefined();
  });

  it("a failed login does not trigger a refresh", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res({ error: { code: "unauthorized", message: "bad" } }, 401));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await expect(
      session.login({ email: "a@b.co", password: "x", device_name: "Chrome", platform: "web" }),
    ).rejects.toMatchObject({ status: 401 });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("logout clears local state even when the server call fails", async () => {
    const fetchMock = vi.fn().mockRejectedValue(new TypeError("offline"));
    const store = signedIn();
    const session = createSession({ baseUrl: "", store, fetch: fetchMock });
    await session.logout(false);
    expect(store.load()).toBeNull();
    expect(session.getState().signedIn).toBe(false);
    expect(JSON.parse(fetchMock.mock.calls[0]![1].body)).toEqual({ all_devices: false });
  });

  it("persists the selected workspace and notifies subscribers", () => {
    const store = signedIn();
    const session = createSession({ baseUrl: "", store, fetch: vi.fn() });
    const listener = vi.fn();
    session.subscribe(listener);
    session.setWorkspaceId("w2");
    expect(session.getState().workspaceId).toBe("w2");
    expect(store.load()?.workspaceId).toBe("w2");
    expect(listener).toHaveBeenCalledTimes(1);
  });

  it("starts signed in from a stored session", () => {
    const session = createSession({ baseUrl: "", store: signedIn({ workspaceId: "w1" }), fetch: vi.fn() });
    expect(session.getState()).toEqual({ signedIn: true, deviceId: "d1", workspaceId: "w1" });
  });
});

describe("session tokens for the realtime client", () => {
  it("exposes the current access token and null once signed out", async () => {
    const fetchMock = vi.fn().mockResolvedValue(res({}));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    expect(session.getAccessToken()).toBe("acc0");
    await session.logout(false);
    expect(session.getAccessToken()).toBeNull();
  });

  it("refreshes on request and says whether a fresh token is available", async () => {
    const fetchMock = vi.fn(async () => res(pair(1)));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await expect(session.refreshAccessToken("acc0")).resolves.toBe(true);
    expect(session.getAccessToken()).toBe("acc1");
    // A token that was already replaced needs no second refresh.
    await expect(session.refreshAccessToken("acc0")).resolves.toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reports false when the refresh was refused, and the session ends", async () => {
    const fetchMock = vi.fn(async () => res({ error: { code: "unauthorized", message: "no" } }, 401));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    await expect(session.refreshAccessToken("acc0")).resolves.toBe(false);
    expect(session.getState().signedIn).toBe(false);
  });

  it("tells token listeners when a refresh replaced the access token", async () => {
    const fetchMock = vi.fn(async () => res(pair(1)));
    const session = createSession({ baseUrl: "", store: signedIn(), fetch: fetchMock });
    const listener = vi.fn();
    const off = session.onTokenChange(listener);
    await session.refreshAccessToken("acc0");
    expect(listener).toHaveBeenCalledTimes(1);
    session.setWorkspaceId("w9"); // not a token change
    expect(listener).toHaveBeenCalledTimes(1);
    off();
    await session.refreshAccessToken("acc1");
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
