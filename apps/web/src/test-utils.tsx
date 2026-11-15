import { createSession, MemoryTokenStore, type Session, type SessionData, type SocketLike } from "@samnak/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { expect, vi } from "vitest";
import { App } from "./App";
import { useLocale } from "./state/locale";
import { RealtimeEnvironmentProvider } from "./state/chat";
import { SessionProvider } from "./state/session";

export function jsonRes(body: unknown, status = 200): Response {
  return new Response(status === 204 ? null : JSON.stringify(body), { status });
}

export function errorRes(status: number, code: string) {
  return jsonRes({ error: { code, message: "server text" } }, status);
}

type Reply = Response | object | undefined | void;
export type Body = Record<string, unknown>;
export type Routes = Record<string, (body: Body, path: string) => Reply | Promise<Reply>>;

export interface Call {
  method: string;
  path: string;
  body: Body;
  headers: Record<string, string>;
}

/**
 * A fake API keyed by "METHOD /path" (no query string). A handler returns a Response or a plain
 * object (sent as 200 JSON); undefined means 204. Unknown routes answer 404.
 */
export function mockServer(routes: Routes) {
  const calls: Call[] = [];
  const fetch = vi.fn(async (url: string, init?: { method?: string; body?: string; headers?: Record<string, string> }) => {
    const path = url.split("?")[0]!;
    const method = init?.method ?? "GET";
    const body = (init?.body ? JSON.parse(init.body) : {}) as Body;
    calls.push({ method, path, body, headers: init?.headers ?? {} });
    const handler = routes[`${method} ${path}`];
    if (!handler) return errorRes(404, "not_found");
    const reply = await handler(body, url);
    if (reply instanceof Response) return reply;
    return reply === undefined ? jsonRes(null, 204) : jsonRes(reply);
  });
  return { fetch, calls, called: (key: string) => calls.filter((c) => `${c.method} ${c.path}` === key) };
}

export const me = {
  id: "u1",
  email: "me@example.com",
  display_name: "Mali",
  avatar_url: "",
  locale: "en",
  timezone: "Asia/Bangkok",
  created_at: "2026-01-01T00:00:00Z",
};

export const acme = { id: "w1", name: "Acme", slug: "acme", role: "owner" };

/** A WebSocket the test plays the server of. */
export class FakeSocket implements SocketLike {
  onopen: (() => void) | null = null;
  onmessage: ((ev: { data: unknown }) => void) | null = null;
  onclose: ((ev: { code: number }) => void) | null = null;
  onerror: ((ev: unknown) => void) | null = null;
  sent: { type: string; data?: unknown }[] = [];
  closed = false;
  handshaken = false;

  constructor(readonly url: string) {}

  send(data: string) {
    this.sent.push(JSON.parse(data));
  }
  close(code?: number) {
    this.closed = true;
    this.onclose?.({ code: code ?? 1005 });
  }
  /** The server accepts the connection and the auth frame. */
  handshake(userId = "u1") {
    this.onopen?.();
    this.receive("ready", { user_id: userId, device_id: "d1" });
    this.handshaken = true;
  }
  /** Delivers a server event; `workspace_id` defaults to the current workspace. */
  receive(type: string, data: object = {}) {
    const withWorkspace = type === "ready" || type === "presence.changed" ? data : { workspace_id: "w1", ...data };
    this.onmessage?.({ data: JSON.stringify({ type, data: withWorkspace }) });
  }
  /** Frames the client sent, of one type. */
  sentOf(type: string) {
    return this.sent.filter((f) => f.type === type);
  }
}

export const baseRoutes: Routes = {
  "GET /api/v1/chat/channels": () => ({ items: [] }),
  "GET /api/v1/auth/me": () => me,
  "GET /api/v1/workspace": () => ({ items: [acme] }),
  "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: [] }),
};

export interface RenderOptions {
  route?: string;
  routes?: Routes;
  /** A stored session, as if the user had signed in before. */
  signedIn?: Partial<SessionData> | false;
}

export interface Channel {
  id: string;
  type: "public" | "private" | "dm";
  name: string;
  topic: string;
  last_seq: number;
  last_read_seq: number;
  unread_count: number;
  mention_count: number;
  member_count: number;
  dm_user_ids: string[];
  archived_at: string | null;
  created_at: string;
}

export function channel(id: string, patch: Partial<Channel> = {}): Channel {
  return {
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
    created_at: "2026-01-01T00:00:00Z",
    ...patch,
  };
}

let nextMessage = 1;

/** A message as the server sends it, with a stable id per call. */
export function message(channelId: string, seq: number, patch: Record<string, unknown> = {}) {
  const n = nextMessage++;
  return {
    id: `m${n}`,
    channel_id: channelId,
    seq,
    sender_id: "u2",
    client_msg_id: `k${n}`,
    kind: "text",
    body: `message ${seq}`,
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
    created_at: "2026-01-01T09:00:00Z",
    ...patch,
  };
}

export const somchai = { user_id: "u2", display_name: "Somchai", email: "som@example.com", role: "member", status: "active", joined_at: "2026-02-01T00:00:00Z" };
export const mali = { user_id: "u1", display_name: "Mali", email: "me@example.com", role: "owner", status: "active", joined_at: "2026-01-01T00:00:00Z" };

/** Routes every chat page needs: the workspace members, who names the senders. */
export const chatRoutes: Routes = {
  "GET /api/v1/workspace/members": () => ({ items: [mali, somchai], next_cursor: "" }),
};

export function renderApp({ route = "/", routes = {}, signedIn = {} }: RenderOptions = {}) {
  useLocale.setState({ locale: "en" });
  const server = mockServer({ ...baseRoutes, ...routes });
  const store = new MemoryTokenStore(
    signedIn === false
      ? null
      : { accessToken: "acc", refreshToken: "ref", deviceId: "d1", workspaceId: "w1", ...signedIn },
  );
  const session: Session = createSession({ baseUrl: "", store, fetch: server.fetch });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const sockets: FakeSocket[] = [];
  const environment = {
    url: "ws://test/api/v1/ws",
    createSocket: (url: string) => {
      const socket = new FakeSocket(url);
      sockets.push(socket);
      return socket;
    },
  };
  const view = render(
    <SessionProvider session={session}>
      <RealtimeEnvironmentProvider value={environment}>
        <QueryClientProvider client={queryClient}>
          <MemoryRouter initialEntries={[route]}>
            <App />
          </MemoryRouter>
        </QueryClientProvider>
      </RealtimeEnvironmentProvider>
    </SessionProvider>,
  );
  /** Waits for the app to open a socket, completes its handshake and returns it. */
  async function live(): Promise<FakeSocket> {
    await waitFor(() => expect(sockets.length).toBeGreaterThan(0));
    const socket = sockets.at(-1)!;
    if (!socket.handshaken) act(() => socket.handshake());
    return socket;
  }
  return { ...view, session, store, sockets, live, ...server };
}

/** The error region is always mounted, so wait for its text rather than for the element. */
export async function expectAlert(text: string) {
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(text));
}
