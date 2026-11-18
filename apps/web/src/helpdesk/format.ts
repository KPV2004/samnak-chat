import type { TicketHistoryEntry, TicketPriority, TicketStatus } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";
import type { T } from "../i18n/useT";

/** The badge look of each status: neutral while new, green once resolved, muted once closed. */
export const statusTones: Record<TicketStatus, { badge: "default" | "secondary" | "outline"; text?: string }> = {
  new: { badge: "secondary" },
  open: { badge: "default" },
  in_progress: { badge: "default" },
  waiting: { badge: "outline" },
  resolved: { badge: "outline", text: "text-success" },
  closed: { badge: "secondary" },
};

/** The badge look of each priority. */
export const priorityTones: Record<TicketPriority, { badge: "default" | "secondary" | "destructive" | "outline" }> = {
  low: { badge: "outline" },
  normal: { badge: "secondary" },
  high: { badge: "default" },
  urgent: { badge: "destructive" },
};

/** The translated names of the statuses and priorities. */
export const statusKeys: Record<TicketStatus, MessageKey> = {
  new: "helpdesk.status.new",
  open: "helpdesk.status.open",
  in_progress: "helpdesk.status.in_progress",
  waiting: "helpdesk.status.waiting",
  resolved: "helpdesk.status.resolved",
  closed: "helpdesk.status.closed",
};

export const priorityKeys: Record<TicketPriority, MessageKey> = {
  low: "helpdesk.priority.low",
  normal: "helpdesk.priority.normal",
  high: "helpdesk.priority.high",
  urgent: "helpdesk.priority.urgent",
};

export const statuses: TicketStatus[] = ["new", "open", "in_progress", "waiting", "resolved", "closed"];
export const priorities: TicketPriority[] = ["low", "normal", "high", "urgent"];

/** The label of a status endpoint move (HD-05), e.g. "waiting" → "Set waiting". */
export const moveKeys: Record<TicketStatus, MessageKey> = {
  new: "helpdesk.move.open",
  open: "helpdesk.move.open",
  in_progress: "helpdesk.move.in_progress",
  waiting: "helpdesk.move.waiting",
  resolved: "helpdesk.move.resolved",
  closed: "helpdesk.move.closed",
};

interface HistoryNames {
  user: (id: string) => string;
  category: (id: string) => string;
}

/**
 * One line of the ticket history (HD-18), in the viewer's language. Category ids are
 * turned into their names; a status or priority value into its translated label.
 */
export function historyLine(entry: TicketHistoryEntry, t: T, names: HistoryNames): string {
  const actor = names.user(entry.actor_id);
  switch (entry.action) {
    case "created":
      return t("helpdesk.history.created", { actor });
    case "assigned":
      return entry.to === ""
        ? t("helpdesk.history.unassigned", { actor })
        : t("helpdesk.history.assigned", { actor, name: names.user(entry.to) });
    case "status_changed":
      return t("helpdesk.history.status_changed", {
        actor,
        from: t(statusKeys[entry.from as TicketStatus] ?? "helpdesk.status.new"),
        to: t(statusKeys[entry.to as TicketStatus] ?? "helpdesk.status.new"),
      });
    case "priority_changed":
      return t("helpdesk.history.priority_changed", {
        actor,
        from: t(priorityKeys[entry.from as TicketPriority] ?? "helpdesk.priority.normal"),
        to: t(priorityKeys[entry.to as TicketPriority] ?? "helpdesk.priority.normal"),
      });
    case "category_changed":
      return t("helpdesk.history.category_changed", {
        actor,
        from: entry.from ? names.category(entry.from) : "—",
        to: entry.to ? names.category(entry.to) : "—",
      });
    case "commented":
      return t("helpdesk.history.commented", { actor });
    default:
      return actor;
  }
}
