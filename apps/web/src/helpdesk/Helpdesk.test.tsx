import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

const create = "POST /api/v1/helpdesk/tickets";

function ticket(patch: Record<string, unknown> = {}) {
  return {
    id: "t1",
    key: "IT-0142",
    number: 142,
    queue_id: "q1",
    category_id: "c1",
    title: "Printer on fire",
    description: "",
    requester_id: "u1",
    assignee_id: null,
    status: "new",
    priority: "normal",
    source_channel_id: null,
    source_message_id: null,
    message_id: null,
    resolved_at: null,
    closed_at: null,
    created_at: "2026-11-17T09:00:00Z",
    updated_at: "2026-11-17T09:00:00Z",
    ...patch,
  };
}

const itQueue = {
  id: "q1",
  name: "IT",
  prefix: "IT",
  channel_id: null,
  agent_ids: ["u2"],
  categories: [
    { id: "c1", name: "Hardware", is_default: true, queue_id: "q1" },
    { id: "c2", name: "Software", is_default: false, queue_id: "q1" },
  ],
};

interface Options {
  /** The caller's workspace role; owners and admins manage the helpdesk. */
  role?: "owner" | "admin" | "member";
  queues?: unknown[];
  tickets?: unknown[];
}

/** The module on, the workspace with the given role, and the ticket and queue calls. */
function routes({ role = "owner", queues = [itQueue], tickets = [ticket()] }: Options = {}, extra: Routes = {}): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["helpdesk"] }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/helpdesk/queues": () => ({ items: queues }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: tickets, next_cursor: "" }),
    [create]: () => ticket({ id: "t9", key: "IT-0143" }),
    ...extra,
  };
}

describe("Helpdesk page", () => {
  it("lists my requests with their key, queue and status", async () => {
    renderApp({ route: "/helpdesk", routes: routes() });
    const list = await screen.findByRole("list", { name: "Tickets" });
    expect(within(list).getByRole("link", { name: "Printer on fire" })).toHaveAttribute("href", "/helpdesk/t1");
    expect(within(list).getByText(/IT-0142/)).toBeInTheDocument();
    expect(within(list).getByText("New")).toBeInTheDocument();
  });

  it("asks the server for the tickets I opened", async () => {
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    await screen.findByRole("list", { name: "Tickets" });
    const url = app.called("GET /api/v1/helpdesk/tickets")[0]!;
    expect(url).toBeDefined();
    expect(app.fetch.mock.calls.map(([u]) => String(u)).some((u) => u.includes("requester=me"))).toBe(true);
  });

  it("says so when I have opened no tickets", async () => {
    renderApp({ route: "/helpdesk", routes: routes({ tickets: [] }) });
    expect(await screen.findByText("You have not opened any tickets yet.")).toBeInTheDocument();
  });

  it("shows the Queue tab to an agent, and its filters reach the server", async () => {
    const user = userEvent.setup();
    const agent = { ...itQueue, agent_ids: ["u1"] };
    const app = renderApp({ route: "/helpdesk", routes: routes({ role: "member", queues: [agent] }) });
    await screen.findByRole("list", { name: "Tickets" });
    await user.click(screen.getByRole("tab", { name: "Queue" }));
    await user.selectOptions(await screen.findByRole("combobox", { name: "Queue" }), "q1");
    await user.selectOptions(screen.getByRole("combobox", { name: "Status" }), "in_progress");
    await user.selectOptions(screen.getByRole("combobox", { name: "Assignee" }), "none");
    await waitFor(() => {
      const urls = app.fetch.mock.calls.map(([u]) => String(u));
      expect(urls.at(-1)).toContain("queue_id=q1");
      expect(urls.at(-1)).toContain("status=in_progress");
      expect(urls.at(-1)).toContain("assignee=none");
    });
  });

  it("hides the Queue tab from a member who is not an agent", async () => {
    renderApp({ route: "/helpdesk", routes: routes({ role: "member" }) });
    await screen.findByRole("list", { name: "Tickets" });
    expect(screen.queryByRole("tab", { name: "Queue" })).not.toBeInTheDocument();
  });

  it("links to the settings for owners and admins only", async () => {
    const owner = renderApp({ route: "/helpdesk", routes: routes({ role: "admin" }) });
    expect(await screen.findByRole("link", { name: "Helpdesk settings" })).toHaveAttribute("href", "/helpdesk/settings");
    owner.unmount();
    renderApp({ route: "/helpdesk", routes: routes({ role: "member" }) });
    await screen.findByRole("list", { name: "Tickets" });
    expect(screen.queryByRole("link", { name: "Helpdesk settings" })).not.toBeInTheDocument();
  });
});

describe("new ticket dialog", () => {
  it("sends the title, the default category and the priority, without empty details", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    await screen.findByRole("list", { name: "Tickets" });
    await user.click(screen.getByRole("button", { name: "New ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.type(within(dialog).getByLabelText("Title"), "Printer on fire");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Priority" }), "high");
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toEqual({ category_id: "c1", title: "Printer on fire", priority: "high" });
    expect(await screen.findByText("Ticket IT-0143 created")).toBeInTheDocument();
  });

  it("sends the details and the category the caller picked", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    await screen.findByRole("list", { name: "Tickets" });
    await user.click(screen.getByRole("button", { name: "New ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.type(within(dialog).getByLabelText("Title"), "VPN drops");
    await user.selectOptions(within(dialog).getByRole("combobox", { name: "Category" }), "c2");
    await user.type(within(dialog).getByLabelText("Details"), "Every hour or so");
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));
    await waitFor(() => expect(app.called(create)).toHaveLength(1));
    expect(app.called(create)[0]!.body).toEqual({
      category_id: "c2",
      title: "VPN drops",
      description: "Every hour or so",
      priority: "normal",
    });
  });

  it("asks for a title and sends nothing", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/helpdesk", routes: routes() });
    await screen.findByRole("list", { name: "Tickets" });
    await user.click(screen.getByRole("button", { name: "New ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    await user.click(within(dialog).getByRole("button", { name: "Send ticket" }));
    expect(await within(dialog).findByText("Enter a title")).toBeInTheDocument();
    expect(app.called(create)).toHaveLength(0);
  });

  it("explains, and cannot send, while no queue takes tickets", async () => {
    const user = userEvent.setup();
    renderApp({ route: "/helpdesk", routes: routes({ queues: [{ ...itQueue, categories: [] }] }) });
    await screen.findByRole("list", { name: "Tickets" });
    await user.click(screen.getByRole("button", { name: "New ticket" }));
    const dialog = await screen.findByRole("dialog", { name: "New ticket" });
    expect(within(dialog).getByText("No queue takes tickets yet. An admin sets them up in Helpdesk settings.")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Send ticket" })).toBeDisabled();
  });
});
