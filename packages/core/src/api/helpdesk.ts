// Helpdesk (M5) types come from the server's OpenAPI document (npm run gen:api);
// this file re-exports them under stable names and holds the calls, the way
// m4-approval.ts does. The attachment and upload calls come with M5-05.
import type { ApiClient } from "./client";
import type { components } from "./schema";

type Schemas = components["schemas"];

export type Ticket = Schemas["HelpdeskTicket"];
export type TicketDetail = Schemas["HelpdeskTicketDetail"];
export type TicketStatus = Ticket["status"];
export type TicketPriority = Ticket["priority"];
export type TicketPage = Schemas["HelpdeskTicketListOutputBody"];
export type TicketComment = Schemas["HelpdeskComment"];
export type TicketHistoryEntry = Schemas["HelpdeskHistoryEntry"];
export type HelpdeskQueue = Schemas["HelpdeskQueue"];
export type HelpdeskCategory = Schemas["HelpdeskCategory"];
export type CreateQueueInput = Schemas["HelpdeskQueueCreateInputBody"];
export type UpdateQueueInput = Schemas["HelpdeskQueueUpdateInputBody"];
export type CreateCategoryInput = Schemas["HelpdeskCategoryCreateInputBody"];
export type UpdateCategoryInput = Schemas["HelpdeskCategoryUpdateInputBody"];
export type CreateTicketInput = Schemas["HelpdeskTicketCreateInputBody"];
export type EditTicketInput = Schemas["HelpdeskTicketEditInputBody"];

export interface ListTicketsParams {
  queue_id?: string;
  /** Any of the statuses; sent comma-separated. */
  status?: TicketStatus[];
  /** `me`, `none`, or a user id. */
  assignee?: string;
  /** `me`: the tickets I opened. */
  requester?: "me";
  /** `next_cursor` of the previous page. */
  cursor?: string;
  limit?: number;
}

export interface HelpdeskApi {
  listQueues(): Promise<{ items: HelpdeskQueue[] }>;
  createQueue(input: CreateQueueInput): Promise<HelpdeskQueue>;
  /** Only the fields given change; `channel_id: null` clears the channel. */
  updateQueue(id: string, patch: UpdateQueueInput): Promise<HelpdeskQueue>;
  /** Replaces the whole agent set. */
  setQueueAgents(id: string, userIds: string[]): Promise<void>;
  createCategory(input: CreateCategoryInput): Promise<HelpdeskCategory>;
  updateCategory(id: string, patch: UpdateCategoryInput): Promise<HelpdeskCategory>;

  listTickets(params?: ListTicketsParams): Promise<TicketPage>;
  createTicket(input: CreateTicketInput): Promise<Ticket>;
  getTicket(id: string): Promise<TicketDetail>;
  assignTicket(id: string, assigneeId: string): Promise<Ticket>;
  changeStatus(id: string, status: TicketStatus): Promise<Ticket>;
  editTicket(id: string, patch: EditTicketInput): Promise<Ticket>;
  /** Internal notes are for agents and admins; the server refuses them to a requester. */
  addComment(id: string, input: { body: string; internal?: boolean }): Promise<TicketComment>;
}

const base = "/api/v1/helpdesk";
const enc = encodeURIComponent;
const json = (method: string, body: unknown) => ({
  method,
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify(body),
});

export function createHelpdeskApi(client: ApiClient): HelpdeskApi {
  return {
    listQueues: () => client.request(`${base}/queues`),
    createQueue: (input) => client.request(`${base}/queues`, json("POST", input)),
    updateQueue: (id, patch) => client.request(`${base}/queues/${enc(id)}`, json("PATCH", patch)),
    setQueueAgents: (id, userIds) => client.request(`${base}/queues/${enc(id)}/agents`, json("PUT", { user_ids: userIds })),
    createCategory: (input) => client.request(`${base}/categories`, json("POST", input)),
    updateCategory: (id, patch) => client.request(`${base}/categories/${enc(id)}`, json("PATCH", patch)),

    listTickets(params = {}) {
      // Listed in the order of `ListTicketsParams`, so the query string is stable.
      const qs = Object.entries({
        queue_id: params.queue_id,
        status: params.status?.join(","),
        assignee: params.assignee,
        requester: params.requester,
        cursor: params.cursor,
        limit: params.limit,
      })
        .filter(([, value]) => value !== undefined && value !== "")
        .map(([key, value]) => `${key}=${enc(String(value))}`)
        .join("&");
      return client.request(`${base}/tickets${qs ? `?${qs}` : ""}`);
    },
    createTicket: (input) => client.request(`${base}/tickets`, json("POST", input)),
    getTicket: (id) => client.request(`${base}/tickets/${enc(id)}`),
    assignTicket: (id, assigneeId) => client.request(`${base}/tickets/${enc(id)}/assign`, json("POST", { assignee_id: assigneeId })),
    changeStatus: (id, status) => client.request(`${base}/tickets/${enc(id)}/status`, json("POST", { status })),
    editTicket: (id, patch) => client.request(`${base}/tickets/${enc(id)}`, json("PATCH", patch)),
    addComment: (id, input) => client.request(`${base}/tickets/${enc(id)}/comments`, json("POST", input)),
  };
}
