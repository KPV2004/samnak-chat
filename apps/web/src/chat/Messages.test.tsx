import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 2, last_read_seq: 0, unread_count: 2 });
const history = [message("c1", 1, { body: "welcome to the channel" }), message("c1", 2, { body: "second message" })];

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: history, has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

const log = () => screen.findByRole("log", { name: "Messages" });

describe("channel view", () => {
  it("shows the newest page as a log region with the senders' names", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    const region = await log();
    expect(await within(region).findByText("welcome to the channel")).toBeInTheDocument();
    expect(within(region).getAllByText("Somchai")).toHaveLength(2);
    expect(within(region).getByText("This is the start of the channel.")).toBeInTheDocument();
    // The first page has no cursor: it is the newest one.
    expect(app.calls.find((c) => c.path.endsWith("/c1/messages"))).toBeDefined();
    const url = app.fetch.mock.calls.map((c) => c[0] as string).find((u) => u.includes("/c1/messages"));
    expect(url).not.toContain("before_seq");
  });

  it("marks the channel read up to the newest message once it is on screen", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await screen.findByText("second message");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/read")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/read")[0]!.body).toEqual({ seq: 2 });
    const nav = screen.getByRole("navigation", { name: "Channels and messages" });
    expect(within(nav).getByRole("link", { name: /general/ })).not.toHaveTextContent("unread");
  });

  it("loads earlier messages upward with before_seq", async () => {
    const user = userEvent.setup();
    const older = [message("c1", 1, { body: "the very first" })];
    const newest = [message("c1", 2, { body: "the newest" })];
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/messages": (_b, url) =>
          url.includes("before_seq=2") ? { items: older, has_more: false } : { items: newest, has_more: true },
      }),
    });
    await screen.findByText("the newest");
    await user.click(await screen.findByRole("button", { name: "Load earlier messages" }));
    const region = await log();
    await within(region).findByText("the very first");
    const texts = within(region).getAllByRole("listitem").map((li) => li.textContent);
    expect(texts[0]).toContain("the very first");
    expect(texts[1]).toContain("the newest");
    expect(app.fetch.mock.calls.some((c) => (c[0] as string).includes("before_seq=2"))).toBe(true);
  });

  it("shows one header for a run of messages from one person, and a divider for each day", async () => {
    const grouped = [
      message("c1", 1, { body: "first", created_at: "2026-01-01T09:00:00Z" }),
      message("c1", 2, { body: "second", created_at: "2026-01-01T09:01:00Z" }),
      message("c1", 3, { body: "next day", created_at: "2026-01-02T09:00:00Z" }),
    ];
    renderApp({
      route: "/chat/c1",
      routes: routes({ "GET /api/v1/chat/channels/c1/messages": () => ({ items: grouped, has_more: false }) }),
    });
    const region = await log();
    await within(region).findByText("first");
    const items = within(region).getAllByRole("listitem");
    expect(items[0]!.querySelector("strong")).toHaveTextContent("Somchai");
    // The second message continues the first: the name is for screen readers only.
    expect(items[1]!.querySelector("strong")).toBeNull();
    expect(items[2]!.querySelector("strong")).toHaveTextContent("Somchai");
    expect(within(region).getAllByRole("separator")).toHaveLength(2);
  });

  it("says the channel is empty", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes({ "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }) }),
    });
    expect(await screen.findByText("No messages yet. Say hello.")).toBeInTheDocument();
  });

  it("does not offer a composer in an archived channel", async () => {
    const archived = { ...general, archived_at: "2026-03-01T00:00:00Z" };
    renderApp({ route: "/chat/c1", routes: routes({ "GET /api/v1/chat/channels": () => ({ items: [archived] }) }) });
    expect(await screen.findByText("This channel is archived. No new messages can be sent.")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Message" })).not.toBeInTheDocument();
  });
});

describe("sending", () => {
  function sendRoutes(gate?: Promise<void>, fail?: () => boolean) {
    return routes({
      "POST /api/v1/chat/channels/c1/messages": async (body) => {
        if (gate) await gate;
        if (fail?.()) return new Response("{}", { status: 500 });
        return new Response(
          JSON.stringify(message("c1", 3, { sender_id: "u1", body: body.body, client_msg_id: body.client_msg_id })),
          { status: 201 },
        );
      },
    });
  }

  it("shows the message pending at once and confirmed when the server answers", async () => {
    const user = userEvent.setup();
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const app = renderApp({ route: "/chat/c1", routes: sendRoutes(gate) });
    const box = await screen.findByRole("textbox", { name: "Message" });
    await user.type(box, "hello team{Enter}");
    // Pending: visible before the server has answered, and the box is empty again.
    expect(await screen.findByText("hello team")).toBeInTheDocument();
    expect(screen.getByText("Sending…")).toBeInTheDocument();
    expect(box).toHaveValue("");
    release();
    await waitFor(() => expect(screen.queryByText("Sending…")).not.toBeInTheDocument());
    expect(screen.getAllByText("hello team")).toHaveLength(1);
    const post = app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body;
    expect(post.body).toBe("hello team");
    expect(post.client_msg_id).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[0-9a-f]{4}-[0-9a-f]{12}$/);
  });

  it("Shift+Enter makes a new line and does not send", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: sendRoutes() });
    const box = await screen.findByRole("textbox", { name: "Message" });
    await user.type(box, "line one{Shift>}{Enter}{/Shift}line two");
    expect(box).toHaveValue("line one\nline two");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("line one\nline two");
  });

  it("does not send an empty message", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: sendRoutes() });
    const box = await screen.findByRole("textbox", { name: "Message" });
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    await user.type(box, "   {Enter}");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("counts characters and refuses more than 4,000", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: sendRoutes() });
    const box = await screen.findByRole("textbox", { name: "Message" });
    expect(screen.getByText("0 / 4,000")).toBeInTheDocument();
    await user.type(box, "abc");
    expect(screen.getByText("3 / 4,000")).toBeInTheDocument();
    await user.clear(box);
    await user.click(box);
    await user.paste("x".repeat(4001));
    expect(screen.getByText("4,001 / 4,000")).toBeInTheDocument();
    expect(screen.getByText("A message can have at most 4,000 characters.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    await user.type(box, "{Enter}");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("marks a failed send, and Retry sends it again with the same client_msg_id", async () => {
    const user = userEvent.setup();
    let failing = true;
    const app = renderApp({ route: "/chat/c1", routes: sendRoutes(undefined, () => failing) });
    const box = await screen.findByRole("textbox", { name: "Message" });
    await user.type(box, "will fail{Enter}");
    expect(await screen.findByText("Not sent.")).toBeInTheDocument();
    expect(screen.getByText("will fail")).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByText("Not sent.")).not.toBeInTheDocument());
    expect(screen.getAllByText("will fail")).toHaveLength(1);
    const posts = app.called("POST /api/v1/chat/channels/c1/messages");
    expect(posts).toHaveLength(2);
    expect(posts[1]!.body.client_msg_id).toBe(posts[0]!.body.client_msg_id);
  });

  it("drops a failed message on Discard", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: sendRoutes(undefined, () => true) });
    const box = await screen.findByRole("textbox", { name: "Message" });
    await user.type(box, "never mind{Enter}");
    await user.click(await screen.findByRole("button", { name: "Discard" }));
    expect(screen.queryByText("never mind")).not.toBeInTheDocument();
  });
});

describe("receiving", () => {
  it("shows a message that arrives over the socket, once even if delivered twice", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await screen.findByText("second message");
    const socket = await app.live();
    const incoming = message("c1", 3, { body: "live from Somchai" });
    act(() => {
      socket.receive("message.created", { channel_id: "c1", message: incoming });
      socket.receive("message.created", { channel_id: "c1", message: incoming });
    });
    expect(await screen.findByText("live from Somchai")).toBeInTheDocument();
    expect(screen.getAllByText("live from Somchai")).toHaveLength(1);
  });

  it("badges a channel that is not open when a message arrives", async () => {
    const random = channel("c2", { name: "random" });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels": () => ({ items: [general, random] }),
        "GET /api/v1/chat/channels/c2/messages": () => ({ items: [], has_more: false }),
      }),
    });
    await screen.findByText("second message");
    const socket = await app.live();
    act(() => socket.receive("message.created", { channel_id: "c2", message: message("c2", 1, { body: "psst" }) }));
    const nav = screen.getByRole("navigation", { name: "Channels and messages" });
    await waitFor(() => expect(within(nav).getByRole("link", { name: /random/ })).toHaveTextContent("1 unread"));
  });

  it("shows a banner while reconnecting and catches up after the socket is back", async () => {
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/messages": (_b, url) =>
          url.includes("after_seq=2")
            ? { items: [message("c1", 3, { body: "sent while offline" })], has_more: false }
            : { items: history, has_more: false },
      }),
    });
    await screen.findByText("second message");
    const socket = await app.live();
    act(() => socket.onclose?.({ code: 1006 }));
    expect(await screen.findByText("Connection lost. Reconnecting…")).toBeInTheDocument();
    await waitFor(() => expect(app.sockets).toHaveLength(2), { timeout: 3000 });
    await app.live();
    expect(await screen.findByText("sent while offline")).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText("Connection lost. Reconnecting…")).not.toBeInTheDocument());
  });
});
