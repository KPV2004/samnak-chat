import type { NotificationItem } from "@samnak/core";
import { act, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderApp, type Routes } from "../test-utils";

// Toasts live in one global store that outlives a test, so each test uses its own titles.
const notice = (title: string, patch: Partial<NotificationItem> = {}): NotificationItem => ({
  id: `n-${title}`,
  kind: "task.assigned",
  title,
  body: "Due tomorrow",
  link: "/tasks",
  data: {},
  read_at: null,
  created_at: "2026-01-01T08:00:00Z",
  ...patch,
});

function routes(extra: Routes = {}): Routes {
  return {
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["chat", "notify", "task"] }),
    "GET /api/v1/notify/unread-count": () => ({ count: 2 }),
    "GET /api/v1/notify/notifications": () => ({ items: [], has_more: false }),
    ...extra,
  };
}

describe("notification toasts", () => {
  it("says a notice that arrives outside do-not-disturb", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    const socket = await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    act(() => socket.receive("notification.created", { notification: notice("Report due"), unread_count: 3 }));

    expect(await screen.findByText("Report due")).toBeInTheDocument();
    expect(screen.getByText("Due tomorrow")).toBeInTheDocument();
  });

  it("counts a quiet notice in the bell but shows no toast for it", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    const socket = await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    act(() =>
      socket.receive("notification.created", {
        notification: notice("Quiet notice"),
        unread_count: 3,
        quiet: true,
      }),
    );

    expect(screen.getByRole("button", { name: "Notifications, 3 unread" })).toBeInTheDocument();
    // A notice that is not quiet still toasts, so the silence above is not just a dead listener.
    act(() => socket.receive("notification.created", { notification: notice("Loud notice"), unread_count: 4 }));
    expect(await screen.findByText("Loud notice")).toBeInTheDocument();
    expect(screen.queryByText("Quiet notice")).not.toBeInTheDocument();
  });

  it("says nothing about a notice of another workspace", async () => {
    const app = renderApp({ route: "/", routes: routes() });
    const socket = await app.live();
    await screen.findByRole("button", { name: "Notifications, 2 unread" });

    act(() =>
      socket.receive("notification.created", {
        workspace_id: "w2",
        notification: notice("Other workspace notice"),
        unread_count: 3,
      }),
    );
    act(() => socket.receive("notification.created", { notification: notice("This workspace notice"), unread_count: 4 }));

    expect(await screen.findByText("This workspace notice")).toBeInTheDocument();
    expect(screen.queryByText("Other workspace notice")).not.toBeInTheDocument();
  });
});
