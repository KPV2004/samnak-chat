export type Mark = "bold" | "italic" | "code" | "link";

/** The text after an edit, and what the box should select (a caret when both are equal). */
export interface Edit {
  text: string;
  start: number;
  end: number;
}

const marks: Record<Mark, [string, string]> = {
  bold: ["**", "**"],
  italic: ["_", "_"],
  code: ["`", "`"],
  link: ["[", "](https://)"],
};

/**
 * Puts a markdown mark round the selection (the subset `markdown.tsx` reads). With nothing selected
 * the caret lands between the marks. A link selects its address so it can be typed over.
 */
export function wrapSelection(text: string, start: number, end: number, mark: Mark): Edit {
  const [open, close] = marks[mark];
  const picked = text.slice(start, end);
  const next = text.slice(0, start) + open + picked + close + text.slice(end);
  if (mark === "link" && picked !== "") {
    const at = start + open.length + picked.length + 2;
    return { text: next, start: at, end: at + "https://".length };
  }
  const at = start + open.length;
  return { text: next, start: at, end: at + picked.length };
}

/** Puts `insert` at the selection, replacing it, with the caret after it. */
export function insertAt(text: string, start: number, end: number, insert: string): Edit {
  const at = start + insert.length;
  return { text: text.slice(0, start) + insert + text.slice(end), start: at, end: at };
}

const dayMs = 24 * 60 * 60 * 1000;
const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();

/**
 * The time on a row of the conversation list, as short as it can be: the clock for today, `yesterday`
 * for the day before, the weekday for the rest of the week, then the date (with the year for another year).
 */
export function formatListTime(iso: string, locale: string, yesterday: string, now: Date = new Date()): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const days = Math.round((startOfDay(now) - startOfDay(date)) / dayMs);
  if (days <= 0) return new Intl.DateTimeFormat(locale, { hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(date);
  if (days === 1) return yesterday;
  if (days < 7) return new Intl.DateTimeFormat(locale, { weekday: "short" }).format(date);
  const sameYear = date.getFullYear() === now.getFullYear();
  return new Intl.DateTimeFormat(locale, sameYear ? { day: "numeric", month: "short" } : { day: "numeric", month: "short", year: "numeric" }).format(date);
}
