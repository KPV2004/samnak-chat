import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const firstNew = message("c1", 2, { body: "first new one" });
const secondNew = message("c1", 3, { body: "second new one" });
const alreadyRead = message("c1", 1, { body: "already read" });

/** Two unread messages whose first one is the top of the loaded page, so no line can be drawn yet. */
const partlyRead = channel("c1", { name: "general", last_seq: 3, last_read_seq: 1, unread_count: 2 });

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [partlyRead] }),
    "GET /api/v1/chat/channels/c1": () => partlyRead,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    // The jump asks for the page that ends on the first unread message: before_seq = read + 2 = 3.
    "GET /api/v1/chat/channels/c1/messages": (_b, url) =>
      url.includes("before_seq=3")
        ? { items: [alreadyRead, firstNew], has_more: false }
        : { items: [firstNew, secondNew], has_more: true },
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

/** jsdom has no IntersectionObserver: the line always counts as out of view, so the jump pill shows. */
class OutOfViewObserver {
  constructor(private readonly cb: IntersectionObserverCallback) {}
  observe = () => this.cb([{ isIntersecting: false } as IntersectionObserverEntry], this as unknown as IntersectionObserver);
  unobserve() {}
  disconnect() {}
}

const visible = Object.getOwnPropertyDescriptor(Document.prototype, "visibilityState");

/** The tab is in the background, so the channel stays unread and the jump pill shows. */
function hideTab() {
  Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" });
}

afterEach(() => {
  // `hideTab` sets the value on the document itself, so restore that before the prototype.
  delete (document as { visibilityState?: unknown }).visibilityState;
  if (visible) Object.defineProperty(Document.prototype, "visibilityState", visible);
  delete (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView;
  vi.unstubAllGlobals();
});

describe("jump to unread", () => {
  it("asks for the page that ends on the first unread message (read seq + 2) and draws the line above it", async () => {
    hideTab();
    vi.stubGlobal("IntersectionObserver", OutOfViewObserver);
    const app = renderApp({ route: "/chat/c1", routes: routes() });

    const region = await screen.findByRole("log", { name: "Messages" });
    await within(region).findByText("first new one");
    // The line cannot be drawn yet: the first loaded message is already unread.
    expect(within(region).queryByRole("separator", { name: "New messages" })).not.toBeInTheDocument();

    fireEvent.click(await screen.findByRole("button", { name: "2 new messages" }));

    // The page ends on the first unread message, not before it: read + 2, exclusive.
    await waitFor(() =>
      expect(app.fetch.mock.calls.map((c) => c[0] as string).some((u) => u.includes("before_seq=3"))).toBe(true),
    );
    const line = await within(region).findByRole("separator", { name: "New messages" });
    expect(line.closest("li")).toHaveTextContent("first new one");
  });

  it("jumps to the line it already has and puts the focus on it, without asking the server again", async () => {
    hideTab();
    vi.stubGlobal("IntersectionObserver", OutOfViewObserver);
    const scrollIntoView = vi.fn();
    (Element.prototype as { scrollIntoView?: unknown }).scrollIntoView = scrollIntoView;
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/messages": () => ({ items: [alreadyRead, firstNew, secondNew], has_more: false }),
      }),
    });

    const region = await screen.findByRole("log", { name: "Messages" });
    await within(region).findByText("second new one");
    const line = within(region).getByRole("separator", { name: "New messages" });

    fireEvent.click(await screen.findByRole("button", { name: "2 new messages" }));

    await waitFor(() => expect(document.activeElement).toBe(line));
    expect(scrollIntoView).toHaveBeenCalled();
    expect(screen.getByText("Jumped to 2 unread messages")).toBeInTheDocument();
    expect(app.fetch.mock.calls.map((c) => c[0] as string).some((u) => u.includes("before_seq"))).toBe(false);
  });

  it("keeps the jump pill after opening the channel, even though opening marks it read (BT-3)", async () => {
    vi.stubGlobal("IntersectionObserver", OutOfViewObserver);
    const app = renderApp({ route: "/chat/c1", routes: routes() });

    const region = await screen.findByRole("log", { name: "Messages" });
    await within(region).findByText("first new one");

    // The open channel is marked read at once (the tab is visible), so the live count is 0.
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/read")).toHaveLength(1));
    // The counts taken when the channel opened still drive the pill, although the live count is 0.
    expect(await screen.findByRole("button", { name: "2 new messages" })).toBeInTheDocument();
  });

  it("goes to the top when nothing was read yet, since the whole channel is new", async () => {
    vi.stubGlobal("IntersectionObserver", OutOfViewObserver);
    const neverRead = channel("c1", { name: "general", last_seq: 2, last_read_seq: 0, unread_count: 2 });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels": () => ({ items: [neverRead] }),
        "GET /api/v1/chat/channels/c1": () => neverRead,
        "GET /api/v1/chat/channels/c1/messages": () => ({ items: [firstNew, secondNew], has_more: false }),
      }),
    });

    const log = await screen.findByRole("log", { name: "Messages" });
    await within(log).findByText("second new one");
    fireEvent.click(await screen.findByRole("button", { name: "2 new messages" }));

    await waitFor(() => expect(screen.queryByRole("button", { name: "2 new messages" })).not.toBeInTheDocument());
    expect((log as HTMLElement).scrollTop).toBe(0);
    // No page is asked for: the top is already loaded and the line has no place above message 1.
    expect(app.fetch.mock.calls.map((c) => c[0] as string).some((u) => u.includes("before_seq"))).toBe(false);
  });
});
