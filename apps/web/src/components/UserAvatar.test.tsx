import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { initialOf, UserAvatar } from "./UserAvatar";

describe("initialOf", () => {
  it("takes the first letter, upper-cased", () => {
    expect(initialOf("mali")).toBe("M");
    expect(initialOf("  Somchai Jaidee")).toBe("S");
  });

  it("skips a Thai leading vowel so the consonant shows", () => {
    expect(initialOf("เมฆา")).toBe("ม");
    expect(initialOf("สมชาย")).toBe("ส");
  });

  it("keeps the tone marks and vowels that sit on the consonant", () => {
    expect(initialOf("ก้อง")).toBe("ก้");
  });

  it("falls back to a placeholder for an empty name", () => {
    expect(initialOf("")).toBe("?");
    expect(initialOf("   ")).toBe("?");
  });
});

describe("UserAvatar", () => {
  it("shows the initial and hides it from screen readers, since the name is shown next to it", () => {
    render(<UserAvatar name="Mali" />);
    expect(screen.getByText("M")).toHaveAttribute("aria-hidden", "true");
  });
});
