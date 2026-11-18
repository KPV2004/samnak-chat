/** Consecutive messages from one person this close together share one header (Slack-style). */
export const runGapMs = 5 * 60_000;

export interface Row<T> {
  message: T;
  /** Same person as the message above, shortly after it: no header of its own. */
  continued: boolean;
  /** First message of its local day: a day divider goes above it. */
  newDay: boolean;
}

interface Sent {
  sender_id: string;
  created_at: string;
}

function dayKey(iso: string): string {
  return new Date(iso).toDateString();
}

/** Marks each message as a continuation of the one above, and as the start of a new day. */
export function toRows<T extends Sent>(messages: T[]): Row<T>[] {
  return messages.map((message, i) => {
    const prev = messages[i - 1];
    if (!prev) return { message, continued: false, newDay: true };
    const newDay = dayKey(prev.created_at) !== dayKey(message.created_at);
    const gap = Date.parse(message.created_at) - Date.parse(prev.created_at);
    const continued = !newDay && prev.sender_id === message.sender_id && gap >= 0 && gap < runGapMs;
    return { message, continued, newDay };
  });
}

/** The label of a day divider: the word for today, otherwise the date, e.g. "Thu, Jan 1, 2026". */
export function dayLabel(iso: string, locale: string, today: string, now: Date = new Date()): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  if (date.toDateString() === now.toDateString()) return today;
  return new Intl.DateTimeFormat(locale, { weekday: "short", day: "numeric", month: "short", year: "numeric" }).format(date);
}
