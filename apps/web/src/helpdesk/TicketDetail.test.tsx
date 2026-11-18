import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { acme, chatRoutes, renderApp, type Routes } from "../test-utils";

const ticketPath = "GET /api/v1/helpdesk/tickets/t1";
const assign = "POST /api/v1/helpdesk/tickets/t1/assign";
const status = "POST /api/v1/helpdesk/tickets/t1/status";
const comment = "POST /api/v1/helpdesk/tickets/t1/comments";

const queue = {
  id: "q1",
  name: "IT",
  prefix: "IT",
  channel_id: null,
  agent_ids: ["u1"],
  categories: [{ id: "c1", name: "Hardware", is_default: true, queue_id: "q1" }],
};

function detail(patch: Record<string, unknown> = {}) {
  return {
    id: "t1",
    key: "IT-0142",
    number: 142,
    queue_id: "q1",
    category_id: "c1",
    title: "Printer on fire",
    description: "Smoke from the tray",
    requester_id: "u2",
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
    comments: [],
    history: [],
    attachments: [],
    ...patch,
  };
}

function routes(
  detailBody: object,
  extra: Routes = {},
  role = "member",
  queues: unknown[] = [queue],
): Routes {
  return {
    ...chatRoutes,
    "GET /api/v1/meta": () => ({ name: "samnak-chat", version: "x", modules: ["helpdesk"] }),
    "GET /api/v1/workspace": () => ({ items: [{ ...acme, role }] }),
    "GET /api/v1/helpdesk/queues": () => ({ items: queues }),
    "GET /api/v1/helpdesk/tickets": () => ({ items: [detail()], next_cursor: "" }),
    [ticketPath]: () => detailBody,
    ...extra,
  };
}

describe("ticket detail", () => {
  it("lets an agent of the queue claim an unassigned ticket", async () => {
    const user = userEvent.setup();
    const app = renderApp({ route: "/helpdesk/t1", routes: routes(detail()) });
    await screen.findByRole("heading", { name: /Printer on fire/ });
    expect(screen.getByText("IT-0142")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Claim" }));
    await waitFor(() => expect(app.called(assign)).toHaveLength(1));
    expect(app.called(assign)[0]!.body).toEqual({ assignee_id: "u1" });
  });

  it("offers the requester only the moves of the state table", async () => {
    const user = userEvent.setup();
    const resolved = detail({
      requester_id: "u1",
      assignee_id: "u2",
      status: "resolved",
      resolved_at: "2026-11-17T10:00:00Z",
    });
    const app = renderApp({ route: "/helpdesk/t1", routes: routes(resolved) });
    await screen.findByRole("heading", { name: /Printer on fire/ });
    expect(screen.queryByRole("button", { name: "Claim" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Mark resolved" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Close ticket" }));
    await waitFor(() => expect(app.called(status)).toHaveLength(1));
    expect(app.called(status)[0]!.body).toEqual({ status: "closed" });
  });

  it("shows the internal switch to agents only, and posts a public comment", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/helpdesk/t1",
      routes: routes(detail(), { [comment]: () => ({ id: "c9" }) }),
    });
    const box = await screen.findByLabelText("Write a comment");
    expect(screen.getByLabelText("Internal note, for agents only")).toBeInTheDocument();
    await user.type(box, "Checking the tray");
    await user.click(screen.getByRole("button", { name: "Post comment" }));
    await waitFor(() => expect(app.called(comment)).toHaveLength(1));
    expect(app.called(comment)[0]!.body).toEqual({ body: "Checking the tray", internal: false });
  });

  it("hides the internal switch from the requester", async () => {
    renderApp({
      route: "/helpdesk/t1",
      // The requester is not an agent of the queue.
      routes: routes(detail({ requester_id: "u1" }), {}, "member", [{ ...queue, agent_ids: [] }]),
    });
    await screen.findByLabelText("Write a comment");
    expect(screen.queryByLabelText("Internal note, for agents only")).not.toBeInTheDocument();
  });

  it("writes the history in the viewer's words", async () => {
    const withHistory = detail({
      assignee_id: "u2",
      history: [
        { id: "h1", action: "created", actor_id: "u2", from: "", to: "", internal: false, at: "2026-11-17T09:00:00Z" },
        {
          id: "h2",
          action: "status_changed",
          actor_id: "u1",
          from: "open",
          to: "in_progress",
          internal: false,
          at: "2026-11-17T09:30:00Z",
        },
      ],
    });
    renderApp({ route: "/helpdesk/t1", routes: routes(withHistory) });
    const history = await screen.findByRole("heading", { name: "History" });
    const card = history.closest("[data-slot=card]") ?? history.parentElement!;
    expect(await within(card as HTMLElement).findByText("Somchai opened the ticket")).toBeInTheDocument();
    expect(await within(card as HTMLElement).findByText("Mali changed the status from Open to In progress")).toBeInTheDocument();
  });
});

describe("helpdesk settings", () => {
  it("sends a member who is not an admin back to the tickets", async () => {
    renderApp({ route: "/helpdesk/settings", routes: routes(detail(), {}, "member") });
    expect(await screen.findByRole("list", { name: "Tickets" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Create queue" })).not.toBeInTheDocument();
  });

  it("creates a queue with its prefix from the settings page", async () => {
    const user = userEvent.setup();
    const app = renderApp({
      route: "/helpdesk/settings",
      routes: routes(detail(), { "POST /api/v1/helpdesk/queues": () => ({ id: "q2" }) }, "admin"),
    });
    await user.type(await screen.findByLabelText("Name"), "HR");
    await user.type(screen.getByLabelText("Ticket prefix"), "HR");
    await user.click(screen.getByRole("button", { name: "Create queue" }));
    await waitFor(() => expect(app.called("POST /api/v1/helpdesk/queues")).toHaveLength(1));
    expect(app.called("POST /api/v1/helpdesk/queues")[0]!.body).toEqual({ name: "HR", prefix: "HR" });
  });
});
