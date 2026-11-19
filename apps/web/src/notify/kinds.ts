import type { MessageKey } from "../i18n/messages";

/**
 * The notification kinds a user can switch off in the settings (NOTI-07), with
 * the module that sends each one. A kind is listed only while its module is on.
 */
export const preferenceKinds: { kind: string; module: string; label: MessageKey }[] = [
  { kind: "chat.mention", module: "chat", label: "notify.prefs.kind.chat.mention" },
  { kind: "chat.dm", module: "chat", label: "notify.prefs.kind.chat.dm" },
  { kind: "chat.thread_reply", module: "chat", label: "notify.prefs.kind.chat.thread_reply" },
  { kind: "task.assigned", module: "task", label: "notify.prefs.kind.task.assigned" },
  { kind: "task.due_soon", module: "task", label: "notify.prefs.kind.task.due_soon" },
  { kind: "task.overdue", module: "task", label: "notify.prefs.kind.task.overdue" },
  { kind: "helpdesk.ticket_created", module: "helpdesk", label: "notify.prefs.kind.helpdesk.ticket_created" },
  { kind: "helpdesk.ticket_assigned", module: "helpdesk", label: "notify.prefs.kind.helpdesk.ticket_assigned" },
  { kind: "helpdesk.status_changed", module: "helpdesk", label: "notify.prefs.kind.helpdesk.status_changed" },
  { kind: "helpdesk.comment", module: "helpdesk", label: "notify.prefs.kind.helpdesk.comment" },
  { kind: "approval.requested", module: "approval", label: "notify.prefs.kind.approval.requested" },
  { kind: "approval.approved", module: "approval", label: "notify.prefs.kind.approval.approved" },
  { kind: "approval.rejected", module: "approval", label: "notify.prefs.kind.approval.rejected" },
  { kind: "approval.cancelled", module: "approval", label: "notify.prefs.kind.approval.cancelled" },
];
