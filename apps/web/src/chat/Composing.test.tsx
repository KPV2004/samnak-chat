import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 1 });

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({
      items: [
        { user_id: "u1", display_name: "Mali", joined_at: "2026-01-01T00:00:00Z" },
        { user_id: "u2", display_name: "Somchai", joined_at: "2026-01-01T00:00:00Z" },
        { user_id: "u3", display_name: "Somsak", joined_at: "2026-01-01T00:00:00Z" },
      ],
    }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [message("c1", 1)], has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    "POST /api/v1/chat/channels/c1/messages": (body) =>
      new Response(
        JSON.stringify(message("c1", 2, { sender_id: "u1", body: body.body, client_msg_id: body.client_msg_id })),
        { status: 201 },
      ),
    ...extra,
  };
}

const box = () => screen.findByRole("textbox", { name: "Message" });

describe("mention picker", () => {
  it("offers channel members while typing @ and inserts <@user_id>", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "hello @som");
    const list = await screen.findByRole("listbox", { name: "Mention someone" });
    const options = within(list).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Somchai", "Somsak"]);
    await user.click(options[0]!);
    expect(await box()).toHaveValue("hello <@u2> ");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await user.type(await box(), "welcome{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("hello <@u2> welcome");
  });

  it("works from the keyboard: arrows move, Enter picks instead of sending", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@som");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(await box()).toHaveValue("<@u3> ");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("closes on Escape without sending, and never offers me", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@");
    const list = await screen.findByRole("listbox");
    expect(within(list).queryByRole("option", { name: "Mali" })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    expect(await box()).toHaveValue("@");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("offers @channel and inserts <!channel>", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@chan");
    await user.click(await screen.findByRole("option", { name: /channel/ }));
    expect(await box()).toHaveValue("<!channel> ");
  });

  it("stays out of the way inside a word, such as an e-mail address", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "write to mali@som");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });
});

describe("typing indicator", () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows who is typing in this channel, and not me or other channels", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await box();
    const socket = await app.live();
    act(() => {
      socket.receive("typing", { channel_id: "c1", user_id: "u1" });
      socket.receive("typing", { channel_id: "c9", user_id: "u2" });
    });
    expect(screen.queryByText(/is typing/)).not.toBeInTheDocument();
    act(() => socket.receive("typing", { channel_id: "c1", user_id: "u2" }));
    expect(await screen.findByText("Somchai is typing…")).toBeInTheDocument();
    act(() => socket.receive("typing", { channel_id: "c1", user_id: "u3" }));
    expect(await screen.findByText("Somchai and Unknown member are typing…")).toBeInTheDocument();
  });

  it("clears when that person's message arrives", async () => {
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await box();
    const socket = await app.live();
    act(() => socket.receive("typing", { channel_id: "c1", user_id: "u2" }));
    await screen.findByText("Somchai is typing…");
    act(() => socket.receive("message.created", { channel_id: "c1", message: message("c1", 2, { body: "done typing" }) }));
    await waitFor(() => expect(screen.queryByText("Somchai is typing…")).not.toBeInTheDocument());
  });

  it("expires after a few seconds without another typing event", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await box();
    const socket = await app.live();
    act(() => socket.receive("typing", { channel_id: "c1", user_id: "u2" }));
    await screen.findByText("Somchai is typing…");
    await act(async () => {
      await vi.advanceTimersByTimeAsync(6000);
    });
    expect(screen.queryByText("Somchai is typing…")).not.toBeInTheDocument();
  });

  it("tells the server I am typing, at most once every 3 seconds", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    const input = await box();
    const socket = await app.live();
    await user.type(input, "hello there");
    expect(socket.sentOf("typing")).toEqual([{ type: "typing", data: { channel_id: "c1" } }]);
  });

  it("does not announce typing for an empty box", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    const input = await box();
    const socket = await app.live();
    await user.type(input, "a");
    await user.clear(input);
    await user.type(input, "{Backspace}");
    expect(socket.sentOf("typing")).toHaveLength(1);
  });
});
