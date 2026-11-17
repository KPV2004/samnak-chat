import type { ApprovalHistoryAction, ApprovalStatus, ApprovalType } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";
import type { T } from "../i18n/useT";

export const typeKeys = {
  leave: "approval.type.leave",
  general: "approval.type.general",
} as const satisfies Record<ApprovalType, MessageKey>;

export const statusKeys = {
  pending: "approval.status.pending",
  approved: "approval.status.approved",
  rejected: "approval.status.rejected",
  cancelled: "approval.status.cancelled",
} as const satisfies Record<ApprovalStatus, MessageKey>;

export const actionKeys = {
  created: "approval.action.created",
  approved: "approval.action.approved",
  rejected: "approval.action.rejected",
  cancelled: "approval.action.cancelled",
} as const satisfies Record<ApprovalHistoryAction, MessageKey>;

export const leaveTypeKeys = {
  annual: "approval.leaveType.annual",
  sick: "approval.leaveType.sick",
  personal: "approval.leaveType.personal",
  other: "approval.leaveType.other",
} as const satisfies Record<string, MessageKey>;

export const halfDayKeys = {
  morning: "approval.halfDay.morning",
  afternoon: "approval.halfDay.afternoon",
} as const satisfies Record<string, MessageKey>;

/** A payload value as text; anything that is not a string reads as empty. */
export const textOf = (value: unknown) => (typeof value === "string" ? value : "");

/** One line for a request: its leave range, or its subject for a general request. */
export function summarize(type: ApprovalType, payload: Record<string, unknown>, t: T): string {
  if (type === "general") return textOf(payload.subject);
  const leave = textOf(payload.leave_type);
  const leaveKey = leaveTypeKeys[leave as keyof typeof leaveTypeKeys];
  return `${leaveKey ? t(leaveKey) : leave} · ${textOf(payload.start_date)} – ${textOf(payload.end_date)}`;
}

export interface FieldRow {
  label: string;
  value: string;
  /** Keep the line breaks of a long text. */
  multiline?: boolean;
}

/** The fields of a request as label and value rows, in the order the form asks them. */
export function payloadRows(type: ApprovalType, payload: Record<string, unknown>, t: T): FieldRow[] {
  if (type === "general") {
    const rows: FieldRow[] = [
      { label: t("approval.subject"), value: textOf(payload.subject) },
      { label: t("approval.details"), value: textOf(payload.details), multiline: true },
    ];
    if (textOf(payload.amount)) {
      rows.push({ label: t("approval.amount"), value: `${textOf(payload.amount)} ${textOf(payload.currency)}` });
    }
    return rows;
  }
  const leave = textOf(payload.leave_type);
  const leaveKey = leaveTypeKeys[leave as keyof typeof leaveTypeKeys];
  const half = textOf(payload.half_day);
  const halfKey = halfDayKeys[half as keyof typeof halfDayKeys];
  const rows: FieldRow[] = [
    { label: t("approval.leaveType"), value: leaveKey ? t(leaveKey) : leave },
    { label: t("approval.startDate"), value: textOf(payload.start_date) },
    { label: t("approval.endDate"), value: textOf(payload.end_date) },
  ];
  if (half) rows.push({ label: t("approval.halfDay"), value: halfKey ? t(halfKey) : half });
  rows.push({ label: t("approval.reason"), value: textOf(payload.reason), multiline: true });
  return rows;
}
