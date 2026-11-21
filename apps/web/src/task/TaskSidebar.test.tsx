import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general" });
const ops = channel("c2", { name: "ops" });

function task(id: string, channelId: string) {
  return {
    id,
    channel_id: channelId,
    message_id: "m0",
    source_message_id: null,
    title: `Task ${id}`,
    description: "",
    creator_id: "u2",
    assignee_id: "u1",
    due_at: null,
    priority: "normal",
    status: "todo",
    created_at: "2026-01-01T09:00:00Z",
    updated_at: "2026-01-01T09:00:00Z",
  };
}

function routes(modules = ["chat", "task"]): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules }),
    "GET /api/v1/chat/channels": () => ({ items: [general, ops] }),
    "GET /api/v1/task/items": () => ({ items: [task("t1", "c1"), task("t2", "c1"), task("t3", "c2")], next_cursor: "" }),
  };
}

describe("task sidebar", () => {
  it("lists the four views as links with their filters in the URL, and marks My tasks by default", async () => {
    renderApp({ route: "/tasks", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Task views" });
    expect(within(nav).getByRole("link", { name: "My tasks" })).toHaveAttribute("href", "/tasks");
    expect(within(nav).getByRole("link", { name: "My tasks" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "Due today" })).toHaveAttribute("href", "/tasks?due=today");
    expect(within(nav).getByRole("link", { name: "Overdue" })).toHaveAttribute("href", "/tasks?due=overdue");
    expect(within(nav).getByRole("link", { name: "Done" })).toHaveAttribute("href", "/tasks?status=done");
    // Only this app.
    expect(screen.queryByRole("navigation", { name: "Channels and messages" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Members" })).not.toBeInTheDocument();
  });

  it("sets the page's filters from a view, and marks it", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/tasks", routes: routes() });
    const nav = await screen.findByRole("navigation", { name: "Task views" });
    await user.click(within(nav).getByRole("link", { name: "Overdue" }));
    expect(within(nav).getByRole("link", { name: "Overdue" })).toHaveAttribute("aria-current", "page");
    expect(within(nav).getByRole("link", { name: "My tasks" })).not.toHaveAttribute("aria-current");
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Due" })).toHaveValue("overdue"));
    await user.click(within(nav).getByRole("link", { name: "Done" }));
    await waitFor(() => expect(screen.getByRole("checkbox", { name: "Done" })).toBeChecked());
    expect(screen.getByRole("checkbox", { name: "Doing" })).not.toBeChecked();
  });

  it("lists the channels that have open tasks, with counts, and filters the page by one", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/tasks", routes: routes() });
    const byChannel = await screen.findByRole("navigation", { name: "By channel" });
    const general = await within(byChannel).findByRole("link", { name: /general/ });
    expect(general).toHaveTextContent("2 open");
    expect(general).toHaveAttribute("href", "/tasks?channel=c1");
    expect(within(byChannel).getByRole("link", { name: /ops/ })).toHaveTextContent("1 open");
    await user.click(general);
    expect(general).toHaveAttribute("aria-current", "page");
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Channel" })).toHaveValue("c1"));
  });

  it("is not on the page, and asks for no tasks, when /meta lacks task", async () => {
    renderApp({ route: "/tasks", routes: routes(["chat"]) });
    expect(await screen.findByRole("searchbox", { name: "Search conversations" }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Task views" })).not.toBeInTheDocument();
  });
});
