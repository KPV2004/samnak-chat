export function formatDateTime(iso: string, locale: string, timeZone?: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const options: Intl.DateTimeFormatOptions = { dateStyle: "medium", timeStyle: "short" };
  try {
    return new Intl.DateTimeFormat(locale, { ...options, timeZone }).format(date);
  } catch {
    // an unknown time zone name: fall back to the browser's
    return new Intl.DateTimeFormat(locale, options).format(date);
  }
}

export function slugify(name: string): string {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40);
}

/** The time of a message: just the clock for today, with the date for an older one. */
export function formatMessageTime(iso: string, locale: string, now: Date = new Date()): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const clock: Intl.DateTimeFormatOptions = { hour: "2-digit", minute: "2-digit" };
  const sameDay = date.toDateString() === now.toDateString();
  return new Intl.DateTimeFormat(locale, sameDay ? clock : { day: "numeric", month: "short", year: "numeric", ...clock }).format(date);
}
