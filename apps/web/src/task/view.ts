import type { TaskDue, TaskStatus } from "@samnak/core";
import { useCallback } from "react";
import { useSearchParams } from "react-router";

export const taskStatuses: TaskStatus[] = ["todo", "doing", "done"];
export const taskDues: TaskDue[] = ["overdue", "today", "week"];
/** What "My tasks" shows when the URL names no status: what is still to do. */
export const openStatuses: TaskStatus[] = ["todo", "doing"];

export interface TaskView {
  status: TaskStatus[];
  due: TaskDue | "";
  channelId: string;
}

/** Reads the filters of the Tasks page from a query string: `status` (comma list, empty for all), `due` and `channel`. */
export function parseTaskView(params: URLSearchParams): TaskView {
  const raw = params.get("status");
  const status = raw === null ? openStatuses : taskStatuses.filter((s) => raw.split(",").includes(s));
  const due = params.get("due");
  return {
    status,
    due: taskDues.find((d) => d === due) ?? "",
    channelId: params.get("channel") ?? "",
  };
}

/** The `/tasks` URL of a view. The default view (open tasks, no other filter) is the bare path. */
export function taskViewPath(view: Partial<TaskView>): string {
  const params = new URLSearchParams();
  const status = view.status ?? openStatuses;
  if (status.join(",") !== openStatuses.join(",")) params.set("status", status.join(","));
  if (view.due) params.set("due", view.due);
  if (view.channelId) params.set("channel", view.channelId);
  const qs = params.toString();
  return qs ? `/tasks?${qs}` : "/tasks";
}

export function sameTaskView(a: TaskView, b: Partial<TaskView>): boolean {
  return taskViewPath(a) === taskViewPath(b);
}

/** The filters of the Tasks page, kept in the URL so the sidebar, the toolbar, back and reload agree. */
export function useTaskView(): [TaskView, (next: Partial<TaskView>) => void] {
  const [params, setParams] = useSearchParams();
  const view = parseTaskView(params);
  const set = useCallback(
    (next: Partial<TaskView>) => {
      const merged = { ...parseTaskView(params), ...next };
      const target = new URL(taskViewPath(merged), "http://x");
      setParams(target.searchParams);
    },
    [params, setParams],
  );
  return [view, set];
}
