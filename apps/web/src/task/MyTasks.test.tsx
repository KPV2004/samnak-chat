import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { channel, chatRoutes, renderApp, type Routes } from "../test-utils";

const general = channel("c1", { name: "general", last_seq: 0 });

function task(id: string, patch: Record<string, unknown> = {}) {
  return {
    id,
    channel_id: "c1",
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
    ...patch,
  };
}

const first = task("t1", { title: "Prepare the report", status: "doing", due_at: "2026-02-01T12:00:00Z", priority: "high" });
const second = task("t2", { title: "Send the invoice", status: "done" });

const modules = (names: string[]): Routes => ({
  "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: names }),
});

function routes(extra: Routes = {}, names = ["chat", "task"]): Routes {
  return {
    ...chatRoutes,
    ...modules(names),
    "GET /api/v1/chat/channels": () => ({ items: [general] }),
    "GET /api/v1/chat/channels/c1": () => general,
    "GET /api/v1/chat/channels/c1/members": () => ({ items: [] }),
    "GET /api/v1/task/items": (_b, url) =>
      url.includes("cursor=abc") ? { items: [second], next_cursor: "" } : { items: [first], next_cursor: "abc" },
    ...extra,
  };
}

/** The task list requests made so far, as URLs. */
function taskUrls(app: { fetch: { mock: { calls: unknown[][] } } }) {
  // The sidebar reads the open tasks of every channel with limit=100; the page asks for 20.
  return app.fetch.mock.calls.map((c) => c[0] as string).filter((u) => u.includes("/api/v1/task/items") && !u.includes("limit=100"));
}

describe("My tasks", () => {
  it("lists the tasks assigned to me, with the channel each one is in", async () => {
    const app = renderApp({ route: "/tasks", routes: routes() });
    expect(await screen.findByRole("heading", { name: "My tasks" })).toBeInTheDocument();
    const row = (await screen.findByText("Prepare the report")).closest("li")!;
    expect(row).toHaveTextContent("Doing · High");
    expect(row).toHaveTextContent(/Due Feb 1, 2026/);
    expect(within(row).getByRole("link", { name: "general" })).toHaveAttribute("href", "/chat/c1");
    expect(taskUrls(app)[0]).toContain("assignee=me");
    // With no filter in the URL the page shows what is still open.
    expect(taskUrls(app)[0]).toContain("status=todo%2Cdoing");
  });

  it("reads its filters from the URL, so a sidebar view can set them", async () => {
    const app = renderApp({ route: "/tasks?status=done&due=overdue&channel=c1", routes: routes() });
    await screen.findByText("Prepare the report");
    const url = taskUrls(app)[0]!;
    expect(url).toContain("status=done");
    expect(url).toContain("due=overdue");
    expect(url).toContain("channel_id=c1");
    expect(screen.getByRole("checkbox", { name: "Done" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "Doing" })).not.toBeChecked();
    expect(screen.getByRole("combobox", { name: "Due" })).toHaveValue("overdue");
    expect(screen.getByRole("combobox", { name: "Channel" })).toHaveValue("c1");
  });

  it("filters by status, due date and channel", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/tasks", routes: routes() });
    await screen.findByText("Prepare the report");

    await user.click(screen.getByRole("checkbox", { name: "Doing" }));
    await waitFor(() => expect(taskUrls(app).at(-1)).toMatch(/status=todo(&|$)/));

    await user.selectOptions(screen.getByRole("combobox", { name: "Due" }), "overdue");
    await waitFor(() => expect(taskUrls(app).at(-1)).toContain("due=overdue"));

    await user.selectOptions(screen.getByRole("combobox", { name: "Channel" }), "c1");
    await waitFor(() => expect(taskUrls(app).at(-1)).toContain("channel_id=c1"));
    const latest = taskUrls(app).at(-1)!;
    expect(latest).toContain("assignee=me");
    expect(latest).toMatch(/status=todo(&|$)/);
  });

  it("clears every filter at once from the filter bar", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/tasks", routes: routes() });
    await screen.findByText("Prepare the report");
    await user.click(screen.getByRole("checkbox", { name: "Doing" }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Due" }), "today");
    await user.click(screen.getByRole("button", { name: "Clear filters" }));

    await waitFor(() => expect(taskUrls(app).at(-1)).toContain("status=todo%2Cdoing"));
    expect(taskUrls(app).at(-1)).not.toContain("due=");
    expect(screen.getByRole("checkbox", { name: "Doing" })).toBeChecked();
    expect(screen.getByRole("combobox", { name: "Due" })).toHaveValue("");
    expect(screen.queryByRole("button", { name: "Clear filters" })).not.toBeInTheDocument();
  });

  it("says so when no task matches", async () => {
    renderApp({ route: "/tasks", routes: routes({ "GET /api/v1/task/items": () => ({ items: [], next_cursor: "" }) }) });
    expect(await screen.findByText("No tasks match these filters.")).toBeInTheDocument();
  });

  it("loads the next page after the one shown", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/tasks", routes: routes() });
    await user.click(await screen.findByRole("button", { name: "Load more" }));
    expect(await screen.findByText("Send the invoice")).toBeInTheDocument();
    expect(taskUrls(app).at(-1)).toContain("cursor=abc");
    expect(screen.getByText("Prepare the report")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Load more" })).not.toBeInTheDocument();
  });

  it("leads back home, and asks for no tasks, when /meta lacks task", async () => {
    const app = renderApp({ route: "/tasks", routes: routes({}, ["chat"]) });
    // The module is off, so the route leads home.
    expect(await screen.findByRole("heading", { name: "Home" }, { timeout: 5000 })).toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "My tasks" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "My tasks" })).not.toBeInTheDocument();
    expect(taskUrls(app)).toHaveLength(0);
  });

  it("is reached from the rail", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/", routes: routes() });
    await user.click(await within(await screen.findByRole("navigation", { name: "Apps" })).findByRole("link", { name: "Tasks" }));
    expect(await screen.findByRole("heading", { name: "My tasks" })).toBeInTheDocument();
  });

  it("has no Tasks app on the rail when /meta lacks task", async () => {
    renderApp({ route: "/", routes: routes({}, ["chat"]) });
    const apps = await screen.findByRole("navigation", { name: "Apps" });
    expect(await within(apps).findByRole("link", { name: "Messenger" })).toBeInTheDocument();
    expect(within(apps).queryByRole("link", { name: "Tasks" })).not.toBeInTheDocument();
  });
});
