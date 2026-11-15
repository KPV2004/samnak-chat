import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 5, unread_count: 3, mention_count: 1 });
const dm = channel("c2", { type: "dm", name: "", dm_user_ids: ["u1", "u2"], last_seq: 1, unread_count: 1 });
const random = channel("c3", { name: "random", member_count: 4 });

function routes(extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/chat/channels": (_body, url) =>
      url.includes("scope=public") ? { items: [general, random] } : { items: [general, dm] },
    "GET /api/v1/chat/channels/c1/messages": () => ({ items: [], has_more: false }),
    "GET /api/v1/chat/channels/c3/messages": () => ({ items: [], has_more: false }),
    "GET /api/v1/chat/channels/c3": () => random,
    "GET /api/v1/chat/channels/c3/members": () => ({ items: [] }),
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    ...extra,
  };
}

describe("sidebar", () => {
  it("lists joined channels and direct messages with unread and mention badges", async () => {
    renderApp({ route: "/chat", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Channels and messages" });
    const channelLink = await within(nav).findByRole("link", { name: /general/ });
    expect(channelLink).toHaveTextContent("3 unread");
    expect(channelLink).toHaveTextContent("Mentions: 1");
    // A DM is named after the other person, not the empty channel name.
    const dmLink = await within(nav).findByRole("link", { name: /Somchai/ });
    expect(dmLink).toHaveTextContent("1 unread");
    expect(dmLink).toHaveAttribute("href", "/chat/c2");
  });

  it("tells what to do when there is no channel", async () => {
    renderApp({ route: "/chat", routes: routes({ "GET /api/v1/chat/channels": () => ({ items: [] }) }) });
    expect(await screen.findByText("You have not joined any channel yet.")).toBeInTheDocument();
  });

  it("offers a retry when the channels cannot be loaded", async () => {
    const user = userEvent.setup();
    let failing = true;
    const app = renderApp({
      route: "/chat",
      routes: routes({
        "GET /api/v1/chat/channels": () => {
          if (failing) return new Response("{}", { status: 500 });
          return { items: [general] };
        },
      }),
    });
    expect(await screen.findByText("Could not load your channels.")).toBeInTheDocument();
    failing = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));
    expect(await screen.findByRole("link", { name: /general/ })).toBeInTheDocument();
    expect(app.called("GET /api/v1/chat/channels").length).toBeGreaterThan(1);
  });
});

describe("browse channels", () => {
  it("lists public channels, marks the joined ones and joins another", async () => {
    const user = userEvent.setup();
    const joined = { ...random };
    let isMember = false;
    const app = renderApp({
      route: "/chat/browse",
      routes: routes({
        "GET /api/v1/chat/channels": (_body, url) => {
          if (url.includes("scope=public")) return { items: [general, random] };
          return { items: isMember ? [general, dm, joined] : [general, dm] };
        },
        "POST /api/v1/chat/channels/c3/join": () => {
          isMember = true;
        },
      }),
    });
    const list = await screen.findByRole("list", { name: "Browse channels" });
    expect(await within(list).findByText("random")).toBeInTheDocument();
    expect(within(list).getByText("Joined")).toBeInTheDocument(); // general
    await user.click(within(list).getByRole("button", { name: "Join random" }));
    expect(app.called("POST /api/v1/chat/channels/c3/join")).toHaveLength(1);
    // After joining the channel opens, and shows in the sidebar.
    expect(await screen.findByRole("heading", { name: "random" })).toBeInTheDocument();
    expect(within(screen.getByRole("navigation", { name: "Channels and messages" })).getByRole("link", { name: /random/ })).toBeInTheDocument();
  });

  it("says so when there is nothing to browse", async () => {
    renderApp({ route: "/chat/browse", routes: routes({ "GET /api/v1/chat/channels": () => ({ items: [] }) }) });
    expect(await screen.findByText("There are no public channels yet.")).toBeInTheDocument();
  });
});

describe("create a channel", () => {
  it("creates a public channel with the people chosen and opens it", async () => {
    const user = userEvent.setup();
    const created = channel("c9", { name: "ops", topic: "On call", last_seq: 0 });
    let exists = false;
    const app = renderApp({
      route: "/chat/new",
      routes: routes({
        "GET /api/v1/chat/channels": () => ({ items: exists ? [general, dm, created] : [general, dm] }),
        "POST /api/v1/chat/channels": () => {
          exists = true;
          return new Response(JSON.stringify(created), { status: 201 });
        },
        "GET /api/v1/chat/channels/c9": () => created,
        "GET /api/v1/chat/channels/c9/messages": () => ({ items: [], has_more: false }),
        "GET /api/v1/chat/channels/c9/members": () => ({ items: [] }),
      }),
    });
    await user.type(await screen.findByLabelText("Channel name"), "ops");
    await user.type(screen.getByLabelText("Topic (optional)"), "On call");
    await user.click(await screen.findByRole("checkbox", { name: "Somchai" }));
    await user.click(screen.getByRole("button", { name: "Create channel" }));
    expect(app.called("POST /api/v1/chat/channels")[0]!.body).toEqual({
      type: "public",
      name: "ops",
      topic: "On call",
      member_ids: ["u2"],
    });
    expect(await screen.findByRole("heading", { name: "ops" })).toBeInTheDocument();
  });

  it("makes a private channel on request and asks for a name", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/chat/new", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "Create channel" }));
    expect(await screen.findByText("Enter a channel name")).toBeInTheDocument();
    expect(app.called("POST /api/v1/chat/channels")).toHaveLength(0);
    await user.click(screen.getByRole("radio", { name: /Private/ }));
    await user.type(screen.getByLabelText("Channel name"), "secret");
    await user.click(screen.getByRole("button", { name: "Create channel" }));
    expect(app.called("POST /api/v1/chat/channels")[0]!.body).toMatchObject({ type: "private", name: "secret" });
  });

  it("explains a name that is already taken", async () => {
    const user = userEvent.setup();
    renderApp({
      route: "/chat/new",
      routes: routes({
        "POST /api/v1/chat/channels": () =>
          new Response(JSON.stringify({ error: { code: "conflict", message: "taken" } }), { status: 409 }),
      }),
    });
    await user.type(await screen.findByLabelText("Channel name"), "general");
    await user.click(screen.getByRole("button", { name: "Create channel" }));
    expect(await screen.findByText("A channel with this name already exists.")).toBeInTheDocument();
  });
});

describe("start a direct message", () => {
  it("opens the conversation with the person chosen", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/chat/dm",
      routes: routes({
        "POST /api/v1/chat/dms": () => dm,
        "GET /api/v1/chat/channels/c2": () => dm,
        "GET /api/v1/chat/channels/c2/messages": () => ({ items: [], has_more: false }),
        "GET /api/v1/chat/channels/c2/members": () => ({ items: [] }),
      }),
    });
    // I am not offered myself.
    expect(await screen.findByRole("checkbox", { name: "Somchai" })).toBeInTheDocument();
    expect(screen.queryByRole("checkbox", { name: "Mali" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Start conversation" }));
    expect(await screen.findByText("Choose at least one person.")).toBeInTheDocument();
    await user.click(screen.getByRole("checkbox", { name: "Somchai" }));
    await user.click(screen.getByRole("button", { name: "Start conversation" }));
    expect(app.called("POST /api/v1/chat/dms")[0]!.body).toEqual({ user_ids: ["u2"] });
    expect(await screen.findByRole("heading", { name: "Somchai" })).toBeInTheDocument();
  });
});
