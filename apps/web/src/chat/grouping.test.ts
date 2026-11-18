import { describe, expect, it } from "vitest";
import { dayLabel, toRows } from "./grouping";

const at = (sender: string, time: string) => ({ sender_id: sender, created_at: `2026-01-01T${time}:00Z` });

describe("toRows", () => {
  it("continues a run of one person's messages within five minutes", () => {
    const rows = toRows([at("u1", "09:00"), at("u1", "09:02"), at("u2", "09:03"), at("u2", "09:09")]);
    expect(rows.map((r) => r.continued)).toEqual([false, true, false, false]);
  });

  it("starts a new header after a pause or when the sender changes back", () => {
    const rows = toRows([at("u1", "09:00"), at("u1", "09:06"), at("u2", "09:07"), at("u1", "09:08")]);
    expect(rows.map((r) => r.continued)).toEqual([false, false, false, false]);
  });

  it("puts a day divider above the first message of each day", () => {
    const rows = toRows([
      at("u1", "09:00"),
      at("u1", "10:00"),
      { sender_id: "u1", created_at: "2026-01-02T08:00:00Z" },
    ]);
    expect(rows.map((r) => r.newDay)).toEqual([true, false, true]);
    expect(rows[2]!.continued).toBe(false);
  });
});

describe("dayLabel", () => {
  const now = new Date("2026-01-05T12:00:00Z");

  it("says today for a message from today", () => {
    expect(dayLabel("2026-01-05T08:00:00Z", "en", "Today", now)).toBe("Today");
  });

  it("gives the date of an older day", () => {
    const label = dayLabel("2026-01-01T08:00:00Z", "en", "Today", now);
    expect(label).toContain("2026");
    expect(label).toContain("Jan");
  });
});
