import { describe, expect, it } from "vitest";
import { formatListTime, insertAt, wrapSelection } from "./format";

describe("wrapSelection", () => {
  it("wraps the selection and keeps it selected", () => {
    expect(wrapSelection("hello world", 0, 5, "bold")).toEqual({ text: "**hello** world", start: 2, end: 7 });
    expect(wrapSelection("a b", 2, 3, "italic")).toEqual({ text: "a _b_", start: 3, end: 4 });
  });

  it("puts the caret between the marks when nothing is selected", () => {
    expect(wrapSelection("", 0, 0, "code")).toEqual({ text: "``", start: 1, end: 1 });
    expect(wrapSelection("ab", 1, 1, "link")).toEqual({ text: "a[](https://)b", start: 2, end: 2 });
  });

  it("selects the address of a link made from text", () => {
    const edit = wrapSelection("see docs", 4, 8, "link");
    expect(edit.text).toBe("see [docs](https://)");
    expect(edit.text.slice(edit.start, edit.end)).toBe("https://");
  });
});

describe("insertAt", () => {
  it("replaces the selection and moves the caret after the insert", () => {
    expect(insertAt("abcd", 1, 3, "🎉")).toEqual({ text: "a🎉d", start: 3, end: 3 });
  });
});

describe("formatListTime", () => {
  const now = new Date(2026, 10, 20, 15, 0); // Fri 20 Nov 2026, local time
  const at = (d: number, h = 9, m = 5, month = 10, year = 2026) => new Date(year, month, d, h, m).toISOString();

  it("shows the clock for today, and Yesterday for the day before", () => {
    expect(formatListTime(at(20), "en", "Yesterday", now)).toBe("09:05");
    expect(formatListTime(at(19), "en", "Yesterday", now)).toBe("Yesterday");
  });

  it("shows the weekday within the last week, then the date, with the year only for another year", () => {
    expect(formatListTime(at(16), "en", "Yesterday", now)).toBe("Mon");
    expect(formatListTime(at(5), "en", "Yesterday", now)).toBe("Nov 5");
    expect(formatListTime(at(5, 9, 5, 0, 2026), "en", "Yesterday", now)).toBe("Jan 5");
    expect(formatListTime(at(5, 9, 5, 10, 2025), "en", "Yesterday", now)).toBe("Nov 5, 2025");
  });

  it("returns the text itself when it is not a date", () => {
    expect(formatListTime("never", "en", "Yesterday", now)).toBe("never");
  });
});
