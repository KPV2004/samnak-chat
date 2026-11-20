import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, errorRes, mali, renderApp, somchai, type Routes } from "../test-utils";

const nok = { ...somchai, user_id: "u3", display_name: "Nok", email: "nok@example.com" };
const preecha = { ...somchai, user_id: "u4", display_name: "Preecha", email: "pre@example.com" };
const dara = { ...somchai, user_id: "u5", display_name: "Dara", email: "dara@example.com" };

const last = (sender: string, preview: string, at: string, kind = "text") => ({ id: `m-${at}`, sender_id: sender, kind, preview, created_at: at });

const general = channel("c1", {
  name: "general",
  last_seq: 5,
  unread_count: 3,
  mention_count: 1,
  last_message: last("u2", "Lunch at noon?", "2026-11-20T09:00:00Z"),
});
const secret = channel("c2", { name: "secret", type: "private", last_message: last("u1", "see you there", "2026-11-19T09:00:00Z") });
const direct = channel("c3", { type: "dm", name: "", dm_user_ids: ["u1", "u2"], last_seq: 2, unread_count: 2, last_message: last("u2", "ping", "2026-11-20T10:00:00Z") });
const group = channel("c4", {
  type: "dm",
  name: "",
  dm_user_ids: ["u1", "u2", "u3", "u4", "u5"],
  last_message: last("u3", "I will bring snacks", "2026-11-18T09:00:00Z"),
});
const quiet = channel("c5", { name: "quiet" });
const pinnedOld = channel("c6", { name: "ops", pinned_at: "2026-11-01T00:00:00Z", last_message: last("u2", "all green", "2026-11-10T09:00:00Z") });

function routes(items = [general, secret, direct, group, quiet, pinnedOld], extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/workspace/members": () => ({ items: [mali, somchai, nok, preecha, dara], next_cursor: "" }),
    "GET /api/v1/chat/channels": () => ({ items }),
    ...extra,
  };
}

const nav = () => screen.findByRole("navigation", { name: "Channels and messages" });
const titles = (list: HTMLElement) => within(list).getAllByRole("link").map((a) => a.querySelector("[data-title]")?.textContent);
/** The row (`li`) of the conversation with exactly this title. */
const rowOf = (list: ReturnType<typeof within>, title: string) =>
  list.getAllByRole("link").find((a: HTMLElement) => a.querySelector("[data-title]")?.textContent === title)!.closest("li")!;

describe("conversation list", () => {
  it("shows the pinned chats first, then everything by latest message", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    expect(await list.findByRole("heading", { name: "Pinned" })).toBeInTheDocument();
    expect(titles(await list.findByRole("list", { name: "Pinned" }))).toEqual(["ops"]);
    expect(titles(list.getByRole("list", { name: "Recent" }))).toEqual(["Somchai", "general", "secret", "Somchai, Nok, Preecha, Dara", "quiet"]);
  });

  it("shows who said what: the sender and the text, your own as You, and a hint for an empty chat", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    expect(await list.findByRole("link", { name: /general/ })).toHaveTextContent("Somchai: Lunch at noon?");
    expect(list.getByRole("link", { name: /secret/ })).toHaveTextContent("You: see you there");
    // In a 1:1 chat the other person's name would only repeat the title.
    expect(within(rowOf(list, "Somchai")).getByRole("link")).toHaveTextContent("ping");
    expect(within(rowOf(list, "Somchai")).getByRole("link")).not.toHaveTextContent("Somchai: ping");
    expect(list.getByRole("link", { name: /quiet/ })).toHaveTextContent("No messages yet");
  });

  it("keeps the unread and mention badges", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const link = await within(await nav()).findByRole("link", { name: /general/ });
    expect(link).toHaveTextContent("3 unread");
    expect(link).toHaveTextContent("Mentions: 1");
    expect(link).toHaveAttribute("href", "/chat/c1");
  });

  it("filters by Unread, Groups and Direct, and by search", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    await list.findByRole("link", { name: /general/ });
    const all = list.getByRole("button", { name: "All" });
    expect(all).toHaveAttribute("aria-pressed", "true");

    await user.click(list.getByRole("button", { name: "Unread" }));
    expect(list.getByRole("button", { name: "Unread" })).toHaveAttribute("aria-pressed", "true");
    expect(list.getAllByRole("link")).toHaveLength(2);

    await user.click(list.getByRole("button", { name: "Direct" }));
    expect(list.getAllByRole("link")).toHaveLength(1);

    await user.click(list.getByRole("button", { name: "Groups" }));
    expect(list.getAllByRole("link")).toHaveLength(5); // channels, private channels and group messages

    await user.click(list.getByRole("button", { name: "All" }));
    await user.type(list.getByRole("searchbox", { name: "Search conversations" }), "sec");
    expect(list.getAllByRole("link")).toHaveLength(1);
    expect(list.getByRole("link", { name: /secret/ })).toBeInTheDocument();
    await user.clear(list.getByRole("searchbox"));
    await user.type(list.getByRole("searchbox"), "zzz");
    expect(list.getByText("No conversation matches.")).toBeInTheDocument();
  });

  it("pins from the row's options menu and unpins again", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat", routes: routes(undefined, { "PUT /api/v1/chat/channels/c1/pin": () => undefined, "DELETE /api/v1/chat/channels/c1/pin": () => undefined }) });
    const list = within(await nav());
    await list.findByRole("link", { name: /general/ });
    await user.click(list.getByRole("button", { name: "Options for general" }));
    await user.click(await screen.findByRole("menuitem", { name: "Pin to top" }));
    expect(titles(list.getByRole("list", { name: "Pinned" }))).toEqual(["general", "ops"]);
    await waitFor(() => expect(app.called("PUT /api/v1/chat/channels/c1/pin")).toHaveLength(1));

    await user.click(list.getByRole("button", { name: "Options for general" }));
    await user.click(await screen.findByRole("menuitem", { name: "Unpin" }));
    expect(titles(list.getByRole("list", { name: "Pinned" }))).toEqual(["ops"]);
    await waitFor(() => expect(app.called("DELETE /api/v1/chat/channels/c1/pin")).toHaveLength(1));
  });

  it("opens the same menu from the keyboard, on the options button after the row", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    const row = await list.findByRole("link", { name: /general/ });
    row.focus();
    await user.tab();
    expect(list.getByRole("button", { name: "Options for general" })).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("menuitem", { name: "Pin to top" })).toBeInTheDocument();
    await user.keyboard("{Escape}");
    expect(list.getByRole("button", { name: "Options for general" })).toHaveFocus();
  });

  it("opens the menu on right-click too", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const row = await within(await nav()).findByRole("link", { name: /general/ });
    fireEvent.contextMenu(row);
    expect(await screen.findByRole("menuitem", { name: "Pin to top" })).toBeInTheDocument();
  });

  it("offers Mute in the menu while notify is on, and marks a muted chat", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat",
      routes: routes(undefined, {
        "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["chat", "notify"] }),
        "GET /api/v1/notify/preferences": () => ({ disabled_kinds: [], muted_channel_ids: ["c1"], dnd_start: null, dnd_end: null }),
      }),
    });
    const list = within(await nav());
    const row = await list.findByRole("link", { name: /general/ });
    await waitFor(() => expect(row).toHaveTextContent("Muted"));
    await user.click(list.getByRole("button", { name: "Options for general" }));
    expect(await screen.findByRole("menuitem", { name: "Unmute channel" })).toBeInTheDocument();
  });

  it("says so, as a toast, when the server refuses a 31st pin", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes(undefined, { "PUT /api/v1/chat/channels/c1/pin": () => errorRes(409, "conflict") }) });
    const list = within(await nav());
    await list.findByRole("link", { name: /general/ });
    await user.click(list.getByRole("button", { name: "Options for general" }));
    await user.click(await screen.findByRole("menuitem", { name: "Pin to top" }));
    expect(await screen.findByText("You can pin at most 30 chats. Unpin one first.")).toBeInTheDocument();
    expect(within(await nav()).queryByRole("list", { name: "Pinned" })?.textContent).not.toContain("general");
  });

  it("follows a pin made in another tab, and a new message", async () => {
    const app = renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    await list.findByRole("link", { name: /general/ });
    const socket = await app.live();
    act(() => socket.receive("channel.pin", { channel_id: "c5", pinned_at: "2026-11-20T11:00:00Z" }));
    expect(titles(list.getByRole("list", { name: "Pinned" }))).toEqual(["quiet", "ops"]);
    act(() => socket.receive("channel.pin", { channel_id: "c5", pinned_at: null }));
    expect(titles(list.getByRole("list", { name: "Pinned" }))).toEqual(["ops"]);
  });

  it("starts a chat or a channel from the New chat menu", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    await user.click(await list.findByRole("button", { name: "New chat" }));
    expect(await screen.findByRole("menuitem", { name: "New message" })).toHaveAttribute("href", "/chat/dm");
    expect(screen.getByRole("menuitem", { name: "New channel" })).toHaveAttribute("href", "/chat/new");
    expect(screen.getByRole("menuitem", { name: "Browse channels" })).toHaveAttribute("href", "/chat/browse");
  });

  it("draws a picture for every chat: initials of the other person, stacked pictures with +N for a group, a lock for a private channel", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const list = within(await nav());
    await list.findByRole("link", { name: /general/ });
    expect(rowOf(list, "Somchai").querySelector("[data-slot=avatar]")).toHaveTextContent("S");
    const stacked = rowOf(list, "Somchai, Nok, Preecha, Dara");
    expect(stacked.querySelectorAll("[data-slot=avatar]")).toHaveLength(2);
    expect(stacked).toHaveTextContent("+2");
    expect(rowOf(list, "secret").querySelector("svg.lucide-lock")).not.toBeNull();
    expect(rowOf(list, "general").querySelector("svg.lucide-hash")).not.toBeNull();
  });
});

describe("channel header", () => {
  it("pins the open chat from its options menu, even while notify is off", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/c1",
      routes: routes(undefined, {
        "GET /api/v1/chat/channels/c1": () => general,
        "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
        "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }),
        "PUT /api/v1/chat/channels/c1/pin": () => undefined,
        "DELETE /api/v1/chat/channels/c1/pin": () => undefined,
      }),
    });
    await user.click(await screen.findByRole("button", { name: "Channel options" }));
    await user.click(await screen.findByRole("menuitem", { name: "Pin to top" }));
    await waitFor(() => expect(app.called("PUT /api/v1/chat/channels/c1/pin")).toHaveLength(1));
    // The list shows it under Pinned, and the menu now offers the way back.
    expect(titles(within(await nav()).getByRole("list", { name: "Pinned" }))).toContain("general");
    await user.click(await screen.findByRole("button", { name: "Channel options" }));
    expect(await screen.findByRole("menuitem", { name: "Unpin" })).toBeInTheDocument();
  });
});
