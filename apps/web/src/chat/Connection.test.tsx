import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, channel, chatRoutes, me, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general" });
const sales = channel("s1", { name: "sales-floor" });
const globex = { id: "w2", name: "Globex", slug: "globex", role: "member" };

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }),
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

function tokenPair(n: number) {
  return { access_token: `acc${n}`, refresh_token: `ref${n}`, expires_in: 900, device_id: "d1", user: me };
}

describe("realtime connection", () => {
  it("authenticates with the access token and connects once for the whole session", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat", routes: routes() });
    const socket = await app.live();
    expect(socket.url).toBe("ws://test/api/v1/ws");
    expect(socket.sent[0]).toEqual({ type: "auth", data: { access_token: "acc" } });
    // Moving between pages does not reconnect.
    const apps = screen.getByRole("navigation", { name: "Apps" });
    await user.click(within(apps).getByRole("link", { name: "Workspace" }));
    await screen.findByRole("heading", { name: "Members" });
    await user.click(within(apps).getByRole("link", { name: /Messenger/ }));
    await screen.findByRole("navigation", { name: "Channels and messages" });
    expect(app.sockets).toHaveLength(1);
    expect(socket.closed).toBe(false);
  });

  it("connects from any page, so unread counts keep arriving", async () => {
    const app = renderApp({ route: "/profile", routes: routes() });
    await app.live();
    expect(app.sockets).toHaveLength(1);
  });

  it("re-authenticates with the new token after the access token is refreshed", async () => {
    const app = renderApp({
      route: "/chat",
      routes: routes({ "POST /api/v1/auth/refresh": () => tokenPair(1) }),
    });
    const first = await app.live();
    await act(async () => {
      expect(await app.session.refreshAccessToken("acc")).toBe(true);
    });
    expect(first.closed).toBe(true);
    await waitFor(() => expect(app.sockets).toHaveLength(2));
    const second = await app.live();
    expect(second.sent[0]).toEqual({ type: "auth", data: { access_token: "acc1" } });
  });

  it("refreshes the token and reconnects when the hub rejects it with 4401", async () => {
    const app = renderApp({
      route: "/chat",
      routes: routes({ "POST /api/v1/auth/refresh": () => tokenPair(2) }),
    });
    const first = await app.live();
    act(() => first.onclose?.({ code: 4401 }));
    await waitFor(() => expect(app.sockets.length).toBeGreaterThan(1));
    const next = await app.live();
    expect(next.sent[0]).toEqual({ type: "auth", data: { access_token: "acc2" } });
    expect(app.called("POST /api/v1/auth/refresh")).toHaveLength(1);
  });

  it("closes the connection on sign-out", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat", routes: routes({ "POST /api/v1/auth/logout": () => undefined }) });
    const socket = await app.live();
    await user.click(await screen.findByRole("button", { name: "Account menu" }));
    await user.click(await screen.findByRole("menuitem", { name: "Sign out" }));
    await waitFor(() => expect(socket.closed).toBe(true));
  });
});

describe("workspace switch", () => {
  it("forgets the old workspace's channels and loads the new one's", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat",
      routes: routes({
        "GET /api/v1/workspace": () => ({ items: [acme, globex] }),
      }),
    });
    const nav = await screen.findByRole("navigation", { name: "Channels and messages" });
    expect(await within(nav).findByRole("link", { name: /general/ })).toBeInTheDocument();

    // From now on the server answers for the other workspace.
    app.fetch.mockImplementation(async (url: string, init?: { method?: string; headers?: Record<string, string> }) => {
      // Pages that mount for the new workspace (the app rail, the rows) read these again.
      if (url === "/api/v1/auth/me") return new Response(JSON.stringify(me), { status: 200 });
      if (url === "/api/v1/meta") return new Response(JSON.stringify({ name: "samnak-chat", version: "x", modules: [] }), { status: 200 });
      if (url.startsWith("/api/v1/chat/channels") && init?.headers?.["X-Workspace-ID"] === "w2") {
        return new Response(JSON.stringify({ items: [sales] }), { status: 200 });
      }
      return new Response(JSON.stringify({ items: [] }), { status: 200 });
    });
    await user.click(await screen.findByRole("button", { name: "Globex" }));

    expect(await within(nav).findByRole("link", { name: /sales-floor/ })).toBeInTheDocument();
    expect(within(nav).queryByRole("link", { name: /general/ })).not.toBeInTheDocument();
    const lastList = app.fetch.mock.calls
      .filter((c) => (c[0] as string).startsWith("/api/v1/chat/channels"))
      .at(-1)!;
    expect((lastList[1] as { headers: Record<string, string> }).headers["X-Workspace-ID"]).toBe("w2");
    // The socket stays: it carries every workspace of the user.
    expect(app.sockets).toHaveLength(1);
  });

  it("does not count another workspace's events in this one", async () => {
    const app = renderApp({ route: "/chat", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Channels and messages" });
    await within(nav).findByRole("link", { name: /general/ });
    const socket = await app.live();
    act(() =>
      socket.onmessage?.({
        data: JSON.stringify({
          type: "message.created",
          data: {
            workspace_id: "w2",
            channel_id: "c1",
            message: { id: "x", channel_id: "c1", seq: 1, sender_id: "u2", client_msg_id: "k", kind: "text", body: "elsewhere", payload: null, fallback_text: "", thread_root_id: null, reply_count: 0, attachments: [], reactions: [], mentions: [], mentions_channel: false, edited_at: null, deleted_at: null, created_at: "2026-01-01T00:00:00Z" },
          },
        }),
      }),
    );
    expect(within(nav).getByRole("link", { name: /general/ })).not.toHaveTextContent("unread");
  });
});
