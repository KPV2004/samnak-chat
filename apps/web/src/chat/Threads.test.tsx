import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 4 });
const root = message("c1", 1, { body: "should we ship on Friday?", reply_count: 2 });
const replyA = message("c1", 2, { body: "yes from me", thread_root_id: root.id });
const replyB = message("c1", 3, { body: "second reply", thread_root_id: root.id, sender_id: "u1" });
const other = message("c1", 4, { body: "unrelated chatter" });

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    // Channel pages include the replies; the thread query returns only them.
    "GET /api/v1/chat/channels/c1/messages": (_b, url) => {
      if (url.includes(`thread_root_id=${root.id}`)) return { items: [replyA, replyB], has_more: false };
      // A catch-up asks for what is newer than what it holds, like the server answers.
      const after = Number(/after_seq=(\d+)/.exec(url)?.[1] ?? -1);
      return { items: [root, replyA, replyB, other].filter((m) => m.seq > after), has_more: false };
    },
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

describe("thread on a phone", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("opens the thread as a sheet over the channel when the screen is narrow", async () => {
    vi.stubGlobal("matchMedia", (media: string) => ({
      matches: false,
      media,
      addEventListener() {},
      removeEventListener() {},
    }));
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "2 replies" }));
    const sheet = await screen.findByRole("dialog", { name: "Thread" });
    expect(await within(sheet).findByText("yes from me")).toBeInTheDocument();
    await user.click(within(sheet).getByRole("button", { name: "Close thread" }));
    expect(screen.queryByRole("dialog", { name: "Thread" })).not.toBeInTheDocument();
  });
});

describe("threads", () => {
  it("keeps replies out of the channel and shows how many there are", async () => {
    renderApp({ route: "/chat/c1", routes: routes() });
    const log = await screen.findByRole("log", { name: "Messages" });
    expect(await within(log).findByText("should we ship on Friday?")).toBeInTheDocument();
    expect(within(log).getByText("unrelated chatter")).toBeInTheDocument();
    expect(within(log).queryByText("yes from me")).not.toBeInTheDocument();
    expect(within(log).getByRole("button", { name: "2 replies" })).toBeInTheDocument();
  });

  it("opens the thread from the reply count, with the root and every reply", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "2 replies" }));
    const panel = await screen.findByRole("complementary", { name: "Thread" });
    expect(within(panel).getByText("should we ship on Friday?")).toBeInTheDocument();
    expect(await within(panel).findByText("yes from me")).toBeInTheDocument();
    expect(within(panel).getByText("second reply")).toBeInTheDocument();
    expect(app.fetch.mock.calls.some((c) => (c[0] as string).includes(`thread_root_id=${root.id}`))).toBe(true);
  });

  it("opens from the Reply action and says when there are no replies", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const item = (await screen.findByText("unrelated chatter")).closest("li")!;
    await user.click(within(item).getByRole("button", { name: "Reply in thread" }));
    const panel = await screen.findByRole("complementary", { name: "Thread" });
    expect(within(panel).getByText("No replies yet.")).toBeInTheDocument();
  });

  it("can be opened by its link and closed again", async () => {
    const user = userEvent.setup();
    renderApp({ route: `/chat/c1?thread=${root.id}`, routes: routes() });
    const panel = await screen.findByRole("complementary", { name: "Thread" });
    await user.click(within(panel).getByRole("button", { name: "Close thread" }));
    expect(screen.queryByRole("complementary", { name: "Thread" })).not.toBeInTheDocument();
  });

  it("sends a reply under the root, not into the channel", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: `/chat/c1?thread=${root.id}`,
      routes: routes({
        "POST /api/v1/chat/channels/c1/messages": (body) =>
          new Response(
            JSON.stringify(
              message("c1", 5, {
                sender_id: "u1",
                body: body.body,
                client_msg_id: body.client_msg_id,
                thread_root_id: body.thread_root_id,
              }),
            ),
            { status: 201 },
          ),
      }),
    });
    const panel = await screen.findByRole("complementary", { name: "Thread" });
    await user.type(await within(panel).findByRole("textbox", { name: "Reply" }), "agreed{Enter}");
    expect(await within(panel).findByText("agreed")).toBeInTheDocument();
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body).toMatchObject({
      body: "agreed",
      thread_root_id: root.id,
    });
    // The reply is not added to the channel's own log.
    const log = screen.getByRole("log", { name: "Messages" });
    expect(within(log).queryByText("agreed")).not.toBeInTheDocument();
  });

  it("shows a reply that arrives over the socket and the new count on the root", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "2 replies" }));
    const panel = await screen.findByRole("complementary", { name: "Thread" });
    await within(panel).findByText("second reply");
    const socket = await app.live();
    const reply = message("c1", 5, { body: "late reply", thread_root_id: root.id });
    act(() => {
      socket.receive("message.created", { channel_id: "c1", message: reply });
      socket.receive("message.updated", { channel_id: "c1", message: { ...root, reply_count: 3 } });
    });
    expect(await within(panel).findByText("late reply")).toBeInTheDocument();

    await waitFor(() =>
      expect(within(screen.getByRole("log", { name: "Messages" })).getByRole("button", { name: "3 replies" })).toBeInTheDocument(),
    );
  });
});
