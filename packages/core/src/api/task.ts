// Task types come from the server's OpenAPI document (npm run gen:api); this
// file re-exports them under stable names and holds the calls, the way m3.ts
// does.
import type { ApiClient } from "./client";
import type { components, paths } from "./schema";

type Schemas = components["schemas"];

export type Task = Schemas["Task"];
export type TaskStatus = Task["status"];
export type TaskPriority = Task["priority"];
export type TaskPage = Schemas["TaskPage"];
export type CreateTaskInput = Schemas["CreateTaskRequest"];
export type UpdateTaskInput = Schemas["UpdateTaskRequest"];

type Query = NonNullable<paths["/task/items"]["get"]["parameters"]["query"]>;
export type TaskDue = NonNullable<Query["due"]>;

export interface ListTasksParams {
  /** `me` keeps the tasks assigned to the caller. */
  assignee?: Query["assignee"];
  /** `me` keeps the tasks the caller created. */
  creator?: Query["creator"];
  status?: TaskStatus[];
  channel_id?: string;
  due?: TaskDue;
  /** `next_cursor` of the previous page. */
  cursor?: string;
  /** 1 to 100. */
  limit?: number;
}

export interface TaskApi {
  listTasks(params?: ListTasksParams): Promise<TaskPage>;
  createTask(input: CreateTaskInput): Promise<Task>;
  getTask(id: string): Promise<Task>;
  /** Only the fields given change; `null` clears `assignee_id` and `due_at`. */
  updateTask(id: string, patch: UpdateTaskInput): Promise<Task>;
}

const base = "/api/v1/task";
const enc = encodeURIComponent;
const json = { "Content-Type": "application/json" };

export function createTaskApi(client: ApiClient): TaskApi {
  return {
    listTasks(params = {}) {
      // Listed in the order of `ListTasksParams`, so the query string is stable.
      const qs = Object.entries({
        assignee: params.assignee,
        creator: params.creator,
        status: params.status?.join(","),
        channel_id: params.channel_id,
        due: params.due,
        cursor: params.cursor,
        limit: params.limit,
      })
        .filter(([, value]) => value !== undefined && value !== "")
        .map(([key, value]) => `${key}=${enc(String(value))}`)
        .join("&");
      return client.request(`${base}/items${qs ? `?${qs}` : ""}`);
    },
    createTask: (input) => client.request(`${base}/items`, { method: "POST", headers: json, body: JSON.stringify(input) }),
    getTask: (id) => client.request(`${base}/items/${enc(id)}`),
    updateTask: (id, patch) =>
      client.request(`${base}/items/${enc(id)}`, { method: "PATCH", headers: json, body: JSON.stringify(patch) }),
  };
}
