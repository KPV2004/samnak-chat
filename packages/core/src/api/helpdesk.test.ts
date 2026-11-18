import { describe, expect, it } from "vitest";
import { createApiClient, type FetchInit, type FetchResponse } from "./client";
import { createHelpdeskApi, type HelpdeskQueue, type Ticket } from "./helpdesk";

const ticket: Ticket = {
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
  created_at: "2026-01-01T09:00:00Z",
  updated_at: "2026-01-01T09:00:00Z",
};

const queue: HelpdeskQueue = {
  id: "q1",
  name: "IT",
  prefix: "IT",
  channel_id: null,
  agent_ids: ["u2"],
  categories: [{ id: "c1", name: "Hardware", is_default: true, queue_id: "q1" }],
};

const ok = (body: unknown, status = 200): FetchResponse => ({
  ok: true,
  status,
  statusText: "",
  json: async () => JSON.parse(JSON.stringify(body)),
});

const forbidden = (): FetchResponse => ({
  ok: false,
  status: 403,
  statusText: "forbidden",
  json: async () => ({ error: { code: "forbidden", message: "not yours" } }),
});

function setup() {
  const requests: { line: string; body: unknown }[] = [];
  const fetch = async (url: string, init: FetchInit = {}): Promise<FetchResponse> => {
    requests.push({ line: `${init.method ?? "GET"} ${url}`, body: init.body ? JSON.parse(init.body) : undefined });
    const path = url.split("?")[0]!;
    if (path === "/api/v1/helpdesk/queues" && init.method === "POST") return ok(queue, 201);
    if (path === "/api/v1/helpdesk/queues") return ok({ items: [queue] });
    if (path === "/api/v1/helpdesk/queues/q1") return ok(queue);
    if (path === "/api/v1/helpdesk/queues/q1/agents") return ok(null, 204);
    if (path === "/api/v1/helpdesk/categories") return ok(queue.categories[0], 201);
    if (path === "/api/v1/helpdesk/categories/c1") return ok(queue.categories[0]);
    if (path === "/api/v1/helpdesk/tickets" && init.method === "POST") return ok(ticket, 201);
    if (path === "/api/v1/helpdesk/tickets") return ok({ items: [ticket], next_cursor: "abc" });
    if (path === "/api/v1/helpdesk/tickets/t1/comments") return ok({ id: "k1" }, 201);
    if (path.startsWith("/api/v1/helpdesk/tickets/t1")) return ok({ ...ticket, comments: [], history: [], attachments: [] });
    return forbidden();
  };
  const client = createApiClient({ baseUrl: "", fetch });
  return { helpdesk: createHelpdeskApi(client), requests };
}

describe("helpdesk api", () => {
  it("lists tickets with only the filters that are set, in a stable order", async () => {
    const { helpdesk, requests } = setup();
    const page = await helpdesk.listTickets({ status: ["new", "open"], assignee: "none", cursor: "p2", limit: 20 });
    expect(page.next_cursor).toBe("abc");
    expect(requests[0]!.line).toBe("GET /api/v1/helpdesk/tickets?status=new%2Copen&assignee=none&cursor=p2&limit=20");
  });

  it("sends no query string when no filter is given", async () => {
    const { helpdesk, requests } = setup();
    await helpdesk.listTickets();
    expect(requests[0]!.line).toBe("GET /api/v1/helpdesk/tickets");
  });

  it("lists my requests with requester=me", async () => {
    const { helpdesk, requests } = setup();
    await helpdesk.listTickets({ requester: "me" });
    expect(requests[0]!.line).toBe("GET /api/v1/helpdesk/tickets?requester=me");
  });

  it("creates a ticket as JSON", async () => {
    const { helpdesk, requests } = setup();
    const created = await helpdesk.createTicket({ category_id: "c1", title: "Printer", priority: "high" });
    expect(created.key).toBe("IT-0142");
    expect(requests[0]).toEqual({
      line: "POST /api/v1/helpdesk/tickets",
      body: { category_id: "c1", title: "Printer", priority: "high" },
    });
  });

  it("reads one ticket with its comments and history", async () => {
    const { helpdesk, requests } = setup();
    const detail = await helpdesk.getTicket("t1");
    expect(detail.comments).toEqual([]);
    expect(requests[0]!.line).toBe("GET /api/v1/helpdesk/tickets/t1");
  });

  it("assigns, changes the status and edits a ticket with the paths of the schema", async () => {
    const { helpdesk, requests } = setup();
    await helpdesk.assignTicket("t1", "u2");
    await helpdesk.changeStatus("t1", "in_progress");
    await helpdesk.editTicket("t1", { priority: "urgent" });
    expect(requests.map((r) => r.line)).toEqual([
      "POST /api/v1/helpdesk/tickets/t1/assign",
      "POST /api/v1/helpdesk/tickets/t1/status",
      "PATCH /api/v1/helpdesk/tickets/t1",
    ]);
    expect(requests.map((r) => r.body)).toEqual([{ assignee_id: "u2" }, { status: "in_progress" }, { priority: "urgent" }]);
  });

  it("posts a public comment by default and an internal one when asked", async () => {
    const { helpdesk, requests } = setup();
    await helpdesk.addComment("t1", { body: "Checked the cable" });
    await helpdesk.addComment("t1", { body: "Vendor says Monday", internal: true });
    expect(requests.map((r) => r.body)).toEqual([
      { body: "Checked the cable" },
      { body: "Vendor says Monday", internal: true },
    ]);
  });

  it("manages queues, their agents and categories", async () => {
    const { helpdesk, requests } = setup();
    await helpdesk.createQueue({ name: "IT", prefix: "IT" });
    await helpdesk.updateQueue("q1", { channel_id: null });
    await helpdesk.setQueueAgents("q1", ["u2"]);
    await helpdesk.createCategory({ queue_id: "q1", name: "Hardware", is_default: true });
    await helpdesk.updateCategory("c1", { archived: true });
    expect(requests.map((r) => r.line)).toEqual([
      "POST /api/v1/helpdesk/queues",
      "PATCH /api/v1/helpdesk/queues/q1",
      "PUT /api/v1/helpdesk/queues/q1/agents",
      "POST /api/v1/helpdesk/categories",
      "PATCH /api/v1/helpdesk/categories/c1",
    ]);
    expect(requests[2]!.body).toEqual({ user_ids: ["u2"] });
  });

  it("passes a refusal on as an ApiError", async () => {
    const { helpdesk } = setup();
    await expect(helpdesk.getTicket("denied")).rejects.toMatchObject({ status: 403, code: "forbidden" });
  });
});
