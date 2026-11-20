import { describe, expect, it } from "vitest";
import { insertAt, wrapSelection } from "./format";

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
