import type { TaskStatus } from "@samnak/core";

/** For each status: the bar on a task card, and the Badge that shows the status (its variant and text colour). */
export const taskStatusTones: Record<TaskStatus, { bar: string; badge: "secondary" | "default" | "outline"; text?: string }> = {
  todo: { bar: "border-l-border", badge: "secondary" },
  doing: { bar: "border-l-primary", badge: "default" },
  done: { bar: "border-l-success", badge: "outline", text: "text-success" },
};
