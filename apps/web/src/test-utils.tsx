import { createSession, MemoryTokenStore, type Session, type SessionData } from "@samnak/core";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { expect, vi } from "vitest";
import { App } from "./App";
import { useLocale } from "./state/locale";
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

export const baseRoutes: Routes = {
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
  const view = render(
    <SessionProvider session={session}>
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[route]}>
          <App />
        </MemoryRouter>
      </QueryClientProvider>
    </SessionProvider>,
  );
  return { ...view, session, store, ...server };
}

/** The error region is always mounted, so wait for its text rather than for the element. */
export async function expectAlert(text: string) {
  await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(text));
}
