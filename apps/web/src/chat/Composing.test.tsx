import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { channel, chatRoutes, expectAlert, message, renderApp, type Routes } from "../test-utils";

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

describe("composer tools", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("wraps the selected text in bold, and puts the caret between the marks when nothing is selected", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const input = (await box()) as HTMLTextAreaElement;
    await user.type(input, "hello world");
    input.setSelectionRange(0, 5);
    await user.click(screen.getByRole("button", { name: "Bold" }));
    expect(input).toHaveValue("**hello** world");
    await user.clear(input);
    await user.click(screen.getByRole("button", { name: "Code" }));
    expect(input).toHaveValue("``");
    await user.keyboard("x");
    expect(input).toHaveValue("`x`");
  });

  it("makes a link from the selection and selects the address to type over", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const input = (await box()) as HTMLTextAreaElement;
    await user.type(input, "the docs");
    input.setSelectionRange(4, 8);
    await user.click(screen.getByRole("button", { name: "Link" }));
    expect(input).toHaveValue("the [docs](https://)");
    expect(input.value.slice(input.selectionStart, input.selectionEnd)).toBe("https://");
  });

  it("inserts an emoji at the caret", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat/c1", routes: routes() });
    const input = await box();
    await user.type(input, "well done");
    await user.click(screen.getByRole("button", { name: "Add emoji" }));
    await user.click(await screen.findByRole("button", { name: "🎉" }));
    expect(input).toHaveValue("well done🎉");
  });

  it("uploads a file when it is chosen, shows it, and sends its id with the message", async () => {
    const user = userEvent.setup();
    const put = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", put);
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "POST /api/v1/chat/uploads": () => ({
          attachment_id: "a1",
          upload: { url: "https://files.example/put?sig=1", method: "PUT", headers: { "Content-Type": "application/pdf" }, expires_at: "2026-11-17T10:00:00Z" },
        }),
      }),
    });
    await box();
    const file = new File(["%PDF"], "quote.pdf", { type: "application/pdf" });
    await user.upload(screen.getByLabelText("Attach files"), file);
    const chips = await screen.findByRole("list", { name: "Files to send" });
    expect(await within(chips).findByText("quote.pdf")).toBeInTheDocument();
    expect(app.called("POST /api/v1/chat/uploads")[0]!.body).toEqual({
      channel_id: "c1",
      file_name: "quote.pdf",
      mime_type: "application/pdf",
      size: 4,
    });
    expect(put).toHaveBeenCalledWith("https://files.example/put?sig=1", expect.objectContaining({ method: "PUT", body: file }));
    // A file alone is enough to send.
    await user.click(screen.getByRole("button", { name: "Send" }));
    await waitFor(() => expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(1));
    expect(app.called("POST /api/v1/chat/channels/c1/messages")[0]!.body).toMatchObject({ attachment_ids: ["a1"] });
    await waitFor(() => expect(screen.queryByRole("list", { name: "Files to send" })).not.toBeInTheDocument());
  });

  it("removes a file before sending, and says when storage refuses one", async () => {
    const user = userEvent.setup();
    const put = vi.fn(async () => new Response(null, { status: 200 }));
    vi.stubGlobal("fetch", put);
    const app = renderApp({
      route: "/chat/c1",
      routes: routes({
        "POST /api/v1/chat/uploads": () => ({
          attachment_id: "a1",
          upload: { url: "https://files.example/put", method: "PUT", headers: {}, expires_at: "2026-11-17T10:00:00Z" },
        }),
      }),
    });
    await box();
    await user.upload(screen.getByLabelText("Attach files"), new File(["x"], "a.txt", { type: "text/plain" }));
    await user.click(await screen.findByRole("button", { name: "Remove a.txt" }));
    expect(screen.queryByRole("list", { name: "Files to send" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Send" })).toBeDisabled();

    put.mockResolvedValueOnce(new Response(null, { status: 403 }));
    await user.upload(screen.getByLabelText("Attach files"), new File(["x"], "b.txt", { type: "text/plain" }));
    await expectAlert("That file could not be uploaded.");
    expect(screen.queryByRole("list", { name: "Files to send" })).not.toBeInTheDocument();
    expect(app.called("POST /api/v1/chat/channels/c1/messages")).toHaveLength(0);
  });
});
