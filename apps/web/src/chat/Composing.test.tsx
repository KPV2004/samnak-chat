import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
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
  it("offers channel members while typing @ and inserts @name, and sends <@user_id>", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "hello @som");
    const list = await screen.findByRole("listbox", { name: "Mention someone" });
    const options = within(list).getAllByRole("option");
    expect(options.map((o) => o.textContent)).toEqual(["Somchai", "Somsak"]);
    await user.click(options[0]!);
    expect(await box()).toHaveValue("hello @Somchai ");
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
    await user.type(await box(), "welcome{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("hello <@u2> welcome");
  });

  it("does not double the space after a picked name when I type a space next", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@som");
    await user.click(await screen.findByRole("option", { name: "Somchai" }));
    await user.type(await box(), " welcome");
    expect(await box()).toHaveValue("@Somchai welcome");
  });

  it("works from the keyboard: arrows move, Enter picks instead of sending", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@som");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(await box()).toHaveValue("@Somsak ");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("sends a name with spaces as the member's token", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/members": () => ({
          items: [{ user_id: "u9", display_name: "Mali Wong", joined_at: "2026-01-01T00:00:00Z" }],
        }),
      }),
    });
    await user.type(await box(), "@mali");
    await user.click(await screen.findByRole("option", { name: "Mali Wong" }));
    expect(await box()).toHaveValue("@Mali Wong ");
    await user.type(await box(), "ok{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("<@u9> ok");
  });

  it("sends two members who share a name as their own tokens", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels/c1/members": () => ({
          items: [
            { user_id: "u9", display_name: "Mali Wong", joined_at: "2026-01-01T00:00:00Z" },
            { user_id: "u10", display_name: "Mali Wong", joined_at: "2026-01-01T00:00:00Z" },
          ],
        }),
      }),
    });
    await user.type(await box(), "@mali");
    await user.click(within(await screen.findByRole("listbox")).getAllByRole("option")[0]!);
    await user.type(await box(), "and @mali");
    await user.click(within(await screen.findByRole("listbox")).getAllByRole("option")[1]!);
    expect(await box()).toHaveValue("@Mali Wong and <@u10> ");
    await user.type(await box(), "ok{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("<@u9> and <@u10> ok");
  });

  it("sends a name typed by hand as plain text", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "hi @Somchai later{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("hi @Somchai later");
  });

  it("forgets the picked names once the message is sent", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@som");
    await user.click(await screen.findByRole("option", { name: "Somchai" }));
    await user.type(await box(), "hello{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    await user.type(await box(), "again @Somchai later{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(2));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[1]!.body.body).toBe("again @Somchai later");
  });

  it("counts and limits what is sent, so a run of mentions stops at 4,000", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        // A member whose token is much longer than the name shown.
        "GET /api/v1/chat/channels/c1/members": () => ({
          items: [{ user_id: "01a1dd98-a14f-7338-8445-caa98ac71951", display_name: "Tt", joined_at: "2026-01-01T00:00:00Z" }],
        }),
      }),
    });
    await user.type(await box(), "@tt");
    await user.click(await screen.findByRole("option", { name: "Tt" }));
    // 110 mentions read as 440 characters, and each is sent as its 39-character token plus a space.
    fireEvent.change(await box(), { target: { value: "@Tt ".repeat(110) } });
    expect(screen.getByText("4,400 / 4,000")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("A message can have at most 4,000 characters.");
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });

  it("drops the draft and its picked names when I open another channel", async () => {
    const user = userEvent.setup();
    const random = channel("c2", { name: "random", last_seq: 0 });
    renderApp({
      route: "/chat/c1",
      routes: routes({
        "GET /api/v1/chat/channels": () => ({ items: [general, random] }),
        "GET /api/v1/chat/channels/c2": () => random,
        "GET /api/v1/chat/channels/c2/members": () => ({ items: [] }),
        "GET /api/v1/chat/channels/c2/messages": () => ({ items: [], has_more: false }),
        "POST /api/v1/chat/channels/c2/read": () => undefined,
      }),
    });
    await user.type(await box(), "@som");
    await user.click(await screen.findByRole("option", { name: "Somchai" }));
    expect(await box()).toHaveValue("@Somchai ");
    const nav = screen.getByRole("navigation", { name: "Channels and messages" });
    await user.click(await within(nav).findByRole("link", { name: /random/ }));
    await waitFor(async () => expect(await box()).toHaveValue(""));
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

  it("offers @channel, shows @channel and sends <!channel>", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes() });
    await user.type(await box(), "@chan");
    await user.click(await screen.findByRole("option", { name: /channel/ }));
    expect(await box()).toHaveValue("@channel ");
    await user.type(await box(), "standup{Enter}");
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body.body).toBe("<!channel> standup");
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
