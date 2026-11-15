import { describe, expect, it } from "vitest";
import { formatMessageTime } from "./format";

describe("formatMessageTime", () => {
  const now = new Date("2026-05-10T12:00:00");

  it("shows only the clock for today", () => {
    expect(formatMessageTime("2026-05-10T09:05:00", "en", now)).toMatch(/^0?9:05/);
  });

  it("adds the date for an earlier day", () => {
    const text = formatMessageTime("2026-05-02T09:05:00", "en", now);
    expect(text).toContain("May");
    expect(text).toContain("2026");
  });

  it("gives back text it cannot parse", () => {
    expect(formatMessageTime("yesterday", "en", now)).toBe("yesterday");
  });
});
