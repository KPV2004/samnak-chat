import type { TaskPriority, TaskStatus } from "../api/task";

/**
 * The payload of a `task.item` card (plan, "task module"). A card payload is
 * raw JSON, so it is checked here instead of trusted.
 */
export interface TaskCardPayload {
  task_id: string;
  title: string;
  status: TaskStatus;
  priority: TaskPriority;
  assignee_id: string | null;
  assignee_name: string;
  due_at: string | null;
  /** The task row's `updated_at` in Unix ms; the newest one wins. */
  version: number;
}

const statuses: readonly string[] = ["todo", "doing", "done"] satisfies TaskStatus[];
const priorities: readonly string[] = ["low", "normal", "high"] satisfies TaskPriority[];

export function parseTaskCard(payload: unknown): TaskCardPayload | null {
  if (typeof payload !== "object" || payload === null) return null;
  const p = payload as Record<string, unknown>;
  if (typeof p.task_id !== "string" || typeof p.title !== "string") return null;
  if (typeof p.status !== "string" || !statuses.includes(p.status)) return null;
  if (typeof p.priority !== "string" || !priorities.includes(p.priority)) return null;
  return {
    task_id: p.task_id,
    title: p.title,
    status: p.status as TaskStatus,
    priority: p.priority as TaskPriority,
    assignee_id: typeof p.assignee_id === "string" ? p.assignee_id : null,
    assignee_name: typeof p.assignee_name === "string" ? p.assignee_name : "",
    due_at: typeof p.due_at === "string" ? p.due_at : null,
    version: typeof p.version === "number" ? p.version : 0,
  };
}
