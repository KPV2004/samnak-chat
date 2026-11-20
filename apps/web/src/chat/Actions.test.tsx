import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, channel, chatRoutes, message, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 3 });

function routes(items: object[], extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/messages": () => ({ items, has_more: false }),
    "POST /api/v1/chat/channels/c1/read": () => undefined,
    ...extra,
  };
}

/** Edit and Delete sit in the "More actions" menu of a message's toolbar. */
async function chooseFromMore(user: UserEvent, item: HTMLElement, name: string) {
  await user.click(within(item).getByRole("button", { name: "More actions" }));
  await user.click(await screen.findByRole("menuitem", { name }));
}

const asMember = { "GET /api/v1/workspace": () => ({ items: [{ ...acme, role: "member" }] }) };

describe("message content", () => {
  it("renders markdown and mentions, but no raw HTML", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes([
        message("c1", 1, { body: "**bold** and <@u1> see https://example.com <script>x</script>", mentions: ["u1"] }),
      ]),
    });
    const bold = await screen.findByText("bold");
    expect(bold.tagName).toBe("STRONG");
    expect(screen.getByText("@Mali")).toHaveAttribute("data-mention", "me");
    expect(screen.getByRole("link", { name: "https://example.com" })).toHaveAttribute("href", "https://example.com");
    expect(document.querySelector("script")).toBeNull();
    expect(screen.getByText(/<script>x<\/script>/)).toBeInTheDocument();
  });

  it("shows a deleted message as a placeholder and an edited one with its mark", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes([
        message("c1", 1, { body: "", deleted_at: "2026-01-01T10:00:00Z" }),
        message("c1", 2, { body: "fixed text", edited_at: "2026-01-01T10:00:00Z" }),
      ]),
    });
    expect(await screen.findByText("This message was deleted.")).toBeInTheDocument();
    expect(screen.getByText("(edited)")).toBeInTheDocument();
  });

  it("shows a card as its fallback text, even for a kind it does not know", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes([
        message("c1", 1, {
          kind: "approval.request",
          body: "",
          fallback_text: "Approve leave for Nok (2 days)",
          payload: { status: "pending" },
        }),
      ]),
    });
    expect(await screen.findByText("Approve leave for Nok (2 days)")).toBeInTheDocument();
  });

  it("lists the file names of attachments", async () => {
    renderApp({
      route: "/chat/c1",
      routes: routes([
        message("c1", 1, {
          body: "",
          attachments: [{ id: "a1", file_name: "ใบลา.pdf", size: 1200, mime_type: "application/pdf", is_image: false }],
        }),
      ]),
    });
    expect(await screen.findByText("ใบลา.pdf")).toBeInTheDocument();
  });
});

describe("edit and delete", () => {
  const mine = message("c1", 1, { sender_id: "u1", body: "my message" });
  const theirs = message("c1", 2, { sender_id: "u2", body: "their message" });

  it("edits my own message", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([mine, theirs], {
        [`PATCH /api/v1/chat/messages/${mine.id}`]: (body) => ({
          ...mine,
          body: body.body,
          edited_at: "2026-01-01T11:00:00Z",
        }),
      }),
    });
    const item = (await screen.findByText("my message")).closest("li")!;
    await chooseFromMore(user, item, "Edit");
    const box = within(item).getByRole("textbox", { name: "Edit message" });
    expect(box).toHaveValue("my message");
    await user.clear(box);
    await user.type(box, "better message");
    await user.click(within(item).getByRole("button", { name: "Save" }));
    expect(await screen.findByText("better message")).toBeInTheDocument();
    expect(app.called(`PATCH /api/v1/chat/messages/${mine.id}`)[0]!.body).toEqual({ body: "better message" });
    expect(within(item).getByText("(edited)")).toBeInTheDocument();
  });

  it("shows a mention as the name when editing, and keeps the mention on save", async () => {
    const user = userEvent.setup();
    const withMention = message("c1", 3, { sender_id: "u1", body: "ask <@u2> about it" });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([withMention], {
        [`PATCH /api/v1/chat/messages/${withMention.id}`]: (body) => ({ ...withMention, body: body.body, edited_at: "2026-01-01T11:00:00Z" }),
      }),
    });
    const item = (await screen.findByText("@Somchai")).closest("li")!;
    await chooseFromMore(user, item, "Edit");
    const box = within(item).getByRole("textbox", { name: "Edit message" });
    expect(box).toHaveValue("ask @Somchai about it");
    await user.type(box, " today");
    await user.click(within(item).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(app.called(`PATCH /api/v1/chat/messages/${withMention.id}`)).toHaveLength(1));
    expect(app.called(`PATCH /api/v1/chat/messages/${withMention.id}`)[0]!.body).toEqual({ body: "ask <@u2> about it today" });
  });

  it("saves a message with a mention unchanged when nothing was edited", async () => {
    const user = userEvent.setup();
    const withMention = message("c1", 3, { sender_id: "u1", body: "<!channel> and <@u2>" });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([withMention], {
        [`PATCH /api/v1/chat/messages/${withMention.id}`]: (body) => ({ ...withMention, body: body.body, edited_at: "2026-01-01T11:00:00Z" }),
      }),
    });
    const item = (await screen.findByText("@Somchai")).closest("li")!;
    await chooseFromMore(user, item, "Edit");
    expect(within(item).getByRole("textbox", { name: "Edit message" })).toHaveValue("@channel and @Somchai");
    await user.click(within(item).getByRole("button", { name: "Save" }));
    await waitFor(() => expect(app.called(`PATCH /api/v1/chat/messages/${withMention.id}`)).toHaveLength(1));
    expect(app.called(`PATCH /api/v1/chat/messages/${withMention.id}`)[0]!.body).toEqual({ body: "<!channel> and <@u2>" });
  });

  it("cancels an edit with Escape", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/c1", routes: routes([mine]) });
    const item = (await screen.findByText("my message")).closest("li")!;
    await chooseFromMore(user, item, "Edit");
    await user.type(within(item).getByRole("textbox", { name: "Edit message" }), "x{Escape}");
    expect(within(item).queryByRole("textbox")).not.toBeInTheDocument();
    expect(screen.getByText("my message")).toBeInTheDocument();
    expect(app.calls.some((c) => c.method === "PATCH")).toBe(false);
  });

  it("deletes my message after a confirmation", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([mine], { [`DELETE /api/v1/chat/messages/${mine.id}`]: () => undefined }),
    });
    const item = (await screen.findByText("my message")).closest("li")!;
    await chooseFromMore(user, item, "Delete");
    expect(app.called(`DELETE /api/v1/chat/messages/${mine.id}`)).toHaveLength(0);
    // The question is asked in a dialog, which sits outside the message.
    const dialog = await screen.findByRole("dialog", { name: "Delete this message?" });
    await user.click(within(dialog).getByRole("button", { name: "Confirm" }));
    expect(await screen.findByText("This message was deleted.")).toBeInTheDocument();
    expect(app.called(`DELETE /api/v1/chat/messages/${mine.id}`)).toHaveLength(1);
    expect(await screen.findByText("Message deleted")).toBeInTheDocument();
  });

  it("keeps the message when the deletion is cancelled", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([mine], { [`DELETE /api/v1/chat/messages/${mine.id}`]: () => undefined }),
    });
    const item = (await screen.findByText("my message")).closest("li")!;
    await chooseFromMore(user, item, "Delete");
    await user.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    expect(screen.getByText("my message")).toBeInTheDocument();
    expect(app.called(`DELETE /api/v1/chat/messages/${mine.id}`)).toHaveLength(0);
  });

  it("offers a member no edit or delete on someone else's message", async () => {
    renderApp({ route: "/chat/c1", routes: routes([mine, theirs], asMember) });
    const item = (await screen.findByText("their message")).closest("li")!;
    expect(within(item).queryByRole("button", { name: "More actions" })).not.toBeInTheDocument();
  });

  it("lets an owner delete someone else's message, but not edit it", async () => {
    renderApp({ route: "/chat/c1", routes: routes([theirs]) });
    const user = userEvent.setup();
    const item = (await screen.findByText("their message")).closest("li")!;
    await user.click(within(item).getByRole("button", { name: "More actions" }));
    expect(await screen.findByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("tells when the change could not be made", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat/c1",
      routes: routes([mine], { [`PATCH /api/v1/chat/messages/${mine.id}`]: () => new Response("{}", { status: 500 }) }),
    });
    const item = (await screen.findByText("my message")).closest("li")!;
    await chooseFromMore(user, item, "Edit");
    await user.type(within(item).getByRole("textbox", { name: "Edit message" }), "!");
    await user.click(within(item).getByRole("button", { name: "Save" }));
    expect(await within(item).findByText("That did not work. Try again.")).toBeInTheDocument();
    expect(within(item).getByRole("textbox", { name: "Edit message" })).toHaveValue("my message!");
  });
});

describe("live updates", () => {
  it("applies an edit and a delete that arrive over the socket", async () => {
    const m1 = message("c1", 1, { body: "before" });
    const m2 = message("c1", 2, { body: "to be removed" });
    const app = renderApp({ route: "/chat/c1", routes: routes([m1, m2]) });
    await screen.findByText("before");
    const socket = await app.live();
    act(() => {
      socket.receive("message.updated", {
        channel_id: "c1",
        message: { ...m1, body: "after", edited_at: "2026-01-01T12:00:00Z" },
      });
      socket.receive("message.deleted", {
        channel_id: "c1",
        message: { ...m2, body: "", deleted_at: "2026-01-01T12:01:00Z" },
      });
    });
    expect(await screen.findByText("after")).toBeInTheDocument();
    expect(await screen.findByText("This message was deleted.")).toBeInTheDocument();
    expect(screen.queryByText("to be removed")).not.toBeInTheDocument();
  });

  it("applies a card update to what is shown only through its fallback text", async () => {
    const card = message("c1", 1, { kind: "approval.request", body: "", fallback_text: "Approve?" });
    const app = renderApp({ route: "/chat/c1", routes: routes([card]) });
    await screen.findByText("Approve?");
    const socket = await app.live();
    act(() => socket.receive("card.updated", { channel_id: "c1", message_id: card.id, payload: { status: "approved" } }));
    expect(screen.getByText("Approve?")).toBeInTheDocument();
  });
});

describe("reactions", () => {
  const reacted = message("c1", 1, {
    body: "react to me",
    reactions: [
      { emoji: "👍", count: 2, me: true },
      { emoji: "🎉", count: 1, me: false },
    ],
  });

  it("shows the counts and which one is mine", async () => {
    renderApp({ route: "/chat/c1", routes: routes([reacted]) });
    const mineBtn = await screen.findByRole("button", { name: "👍: 2" });
    expect(mineBtn).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "🎉: 1" })).toHaveAttribute("aria-pressed", "false");
  });

  it("adds my reaction to someone else's emoji, at once", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([reacted], { [`POST /api/v1/chat/messages/${reacted.id}/reactions`]: () => undefined }),
    });
    await user.click(await screen.findByRole("button", { name: "🎉: 1" }));
    expect(await screen.findByRole("button", { name: "🎉: 2" })).toHaveAttribute("aria-pressed", "true");
    expect(app.called(`POST /api/v1/chat/messages/${reacted.id}/reactions`)[0]!.body).toEqual({ emoji: "🎉" });
  });

  it("removes my reaction, and the chip disappears when it was the only one", async () => {
    const user = userEvent.setup();
    const only = message("c1", 1, { body: "x", reactions: [{ emoji: "👀", count: 1, me: true }] });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([only], {
        [`DELETE /api/v1/chat/messages/${only.id}/reactions/${encodeURIComponent("👀")}`]: () => undefined,
      }),
    });
    await user.click(await screen.findByRole("button", { name: "👀: 1" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /👀/ })).not.toBeInTheDocument());
    expect(app.calls.filter((c) => c.method === "DELETE")).toHaveLength(1);
  });

  it("picks a new emoji from the reaction menu", async () => {
    const user = userEvent.setup();
    const plain = message("c1", 1, { body: "no reactions yet" });
    const app = renderApp({
      route: "/chat/c1",
      routes: routes([plain], { [`POST /api/v1/chat/messages/${plain.id}/reactions`]: () => undefined }),
    });
    const item = (await screen.findByText("no reactions yet")).closest("li")!;
    await user.click(within(item).getByRole("button", { name: "Add reaction" }));
    await user.click(within(item).getByRole("button", { name: "❤️" }));
    expect(await within(item).findByRole("button", { name: "❤️: 1" })).toBeInTheDocument();
    expect(app.called(`POST /api/v1/chat/messages/${plain.id}/reactions`)[0]!.body).toEqual({ emoji: "❤️" });
  });

  it("puts the reaction back when the server refuses it", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat/c1",
      routes: routes([reacted], {
        [`POST /api/v1/chat/messages/${reacted.id}/reactions`]: () => new Response("{}", { status: 500 }),
      }),
    });
    await user.click(await screen.findByRole("button", { name: "🎉: 1" }));
    expect(await screen.findByText("That did not work. Try again.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "🎉: 1" })).toHaveAttribute("aria-pressed", "false");
  });
});
