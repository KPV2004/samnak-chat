import type { NotificationItem } from "@samnak/core";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, renderApp, type Routes } from "../test-utils";

const mention: NotificationItem = {
  id: "n1",
  kind: "chat.mention",
  title: "server title",
  body: "hi @Mali",
  link: "/profile",
  data: { channel_id: "c1", message_id: "m1", seq: 3, sender_name: "Somchai", channel_name: "general" },
  read_at: null,
  created_at: "2026-01-01T09:00:00Z",
};

const assigned: NotificationItem = {
  id: "n2",
  kind: "task.assigned",
  title: "Prepare the report",
  body: "Due tomorrow",
  link: "/profile",
  data: {},
  read_at: null,
  created_at: "2026-01-01T08:00:00Z",
};

const older: NotificationItem = {
  id: "n3",
  kind: "chat.thread_reply",
  title: "Mali in a thread",
  body: "one more thing",
  link: "/profile",
  data: { channel_id: "c1", message_id: "m2", seq: 4, thread_root_id: "m0" },
  read_at: null,
  created_at: "2026-01-01T07:00:00Z",
};

function routes(extra: Routes = {}): Routes {
  return {
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["chat", "notify"] }),
    "GET /api/v1/notify/unread-count": () => ({ count: 2 }),
    "GET /api/v1/notify/notifications": () => ({ items: [mention, assigned], has_more: false }),
    "POST /api/v1/notify/notifications/n1/read": () => undefined,
    "POST /api/v1/notify/notifications/n2/read": () => undefined,
    "POST /api/v1/notify/read-all": () => undefined,
    ...extra,
  };
}

describe("the bell", () => {
  it("shows the unread count the socket reports", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    await app.live();
    expect(await screen.findByRole("button", { name: "Notifications, 2 unread" })).toBeInTheDocument();
  });

  it("caps the count at 99+", async () => {
    const app = renderApp({
      route: "/",
      routes: routes({ "GET /api/v1/notify/unread-count": () => ({ count: 120 }) }),
    });
    await app.live();
    const bell = await screen.findByRole("button", { name: "Notifications, more than 99 unread" });
    expect(bell).toHaveTextContent("99+");
  });

  it("hides the count at zero", async () => {
    const app = renderApp({
      route: "/",
      routes: routes({ "GET /api/v1/notify/unread-count": () => ({ count: 0 }) }),
    });
    await app.live();
    const bell = await screen.findByRole("button", { name: "Notifications" });
    expect(bell).not.toHaveTextContent("0");
  });

  it("rises at once when a notification arrives over the socket", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    const socket = await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    act(() => socket.receive("notification.created", { notification: older, unread_count: 3 }));

    expect(screen.getByRole("button", { name: "Notifications, 3 unread" })).toBeInTheDocument();
  });

  it("clears when another device reads everything", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    const socket = await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    act(() => socket.receive("notification.read", { ids: [], all: true, unread_count: 0 }));

    expect(await screen.findByRole("button", { name: "Notifications" })).toBeInTheDocument();
  });

  it("counts the notifications of the workspace on screen", async () => {
    const user = userEvent.setup();
    const beta = { id: "w2", name: "Beta", slug: "beta", role: "owner" };
    let asked = 0;
    const app = renderApp({
      route: "/",
      routes: routes({
        "GET /api/v1/workspace": () => ({ items: [acme, beta] }),
        "GET /api/v1/notify/unread-count": () => ({ count: ++asked === 1 ? 2 : 5 }),
      }),
    });
    await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    await user.click(await screen.findByRole("button", { name: "Workspace Acme" }));
    await user.click(await screen.findByRole("menuitem", { name: "Beta" }));

    expect(await screen.findByRole("button", { name: "Notifications, 5 unread" })).toBeInTheDocument();
    expect(app.called("GET /api/v1/notify/unread-count").at(-1)?.headers["X-Workspace-ID"]).toBe("w2");
  });

  it("is hidden while notify is not an enabled module", async () => {
    const app = renderApp({
      route: "/",
      routes: routes({ "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["chat"] }) }),
    });
    const socket = await app.live();
    await waitFor(() => expect(app.called("GET /api/v1/meta").length).toBeGreaterThan(0));
    // Give a late count request a chance to show up before saying there is none.
    act(() => socket.receive("ready", { user_id: "u1", device_id: "d1" }));
    await waitFor(() => expect(app.called("GET /api/v1/chat/channels").length).toBeGreaterThan(0));
    expect(screen.queryByRole("button", { name: /Notifications/ })).toBeNull();
    expect(app.called("GET /api/v1/notify/unread-count")).toHaveLength(0);
  });
});

describe("the panel", () => {
  it("lists the newest first, labels known kinds and falls back to title and body", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/", routes: routes() });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));

    const panel = await screen.findByRole("region", { name: "Notifications" });
    await waitFor(() => expect(within(panel).getAllByRole("listitem")).toHaveLength(2));
    const items = within(panel).getAllByRole("listitem");
    expect(items[0]).toHaveTextContent("Mentioned you");
    // Chat kinds say who and where from data, in the reader's language.
    expect(items[0]).toHaveTextContent("Somchai in #general");
    expect(items[0]).not.toHaveTextContent("server title");
    expect(items[0]).toHaveTextContent("hi @Mali");
    // An unknown kind shows what the source module wrote.
    expect(items[1]).toHaveTextContent("Prepare the report");
    expect(items[1]).toHaveTextContent("Due tomorrow");
    expect(items[1]).not.toHaveTextContent(/chat\./);
  });

  it("loads the next older page on demand", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/",
      routes: routes({
        "GET /api/v1/notify/notifications": (_body, url) =>
          url.includes("before=n2") ? { items: [older], has_more: false } : { items: [mention, assigned], has_more: true },
      }),
    });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    const panel = await screen.findByRole("region", { name: "Notifications" });

    await user.click(await within(panel).findByRole("button", { name: "Load more" }));

    expect(await within(panel).findByText("Replied in a thread")).toBeInTheDocument();
    expect(within(panel).getAllByRole("listitem")).toHaveLength(3);
    expect(app.called("GET /api/v1/notify/notifications").at(-1)?.headers["X-Workspace-ID"]).toBe("w1");
  });

  it("marks one read and opens its link", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/", routes: routes() });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    const panel = await screen.findByRole("region", { name: "Notifications" });

    await user.click(await within(panel).findByRole("button", { name: /Somchai in #general/ }));

    expect(await screen.findByRole("heading", { name: "Profile" })).toBeInTheDocument();
    expect(app.called("POST /api/v1/notify/notifications/n1/read")).toHaveLength(1);
    // The panel closes with the navigation.
    expect(screen.queryByRole("region", { name: "Notifications" })).toBeNull();
  });

  it("marks everything read", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/", routes: routes() });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    const panel = await screen.findByRole("region", { name: "Notifications" });

    await user.click(await within(panel).findByRole("button", { name: "Mark all as read" }));

    expect(app.called("POST /api/v1/notify/read-all")).toHaveLength(1);
    expect(await screen.findByRole("button", { name: "Notifications" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Mark all as read" })).toBeDisabled();
  });

  it("says so when there is nothing to show", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/",
      routes: routes({
        "GET /api/v1/notify/unread-count": () => ({ count: 0 }),
        "GET /api/v1/notify/notifications": () => ({ items: [], has_more: false }),
      }),
    });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications" }));

    expect(await screen.findByText("You have no notifications.")).toBeInTheDocument();
  });

  it("offers a retry when the list cannot be loaded", async () => {
    const user = userEvent.setup();
    let failing = true;
    const app = renderApp({
      route: "/",
      routes: routes({
        "GET /api/v1/notify/notifications": () => {
          if (failing) return new Response("{}", { status: 500 });
          return { items: [mention], has_more: false };
        },
      }),
    });
    await app.live();
    await user.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    expect(await screen.findByText("Could not load your notifications.")).toBeInTheDocument();

    failing = false;
    await user.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("Somchai in #general")).toBeInTheDocument();
  });
});
