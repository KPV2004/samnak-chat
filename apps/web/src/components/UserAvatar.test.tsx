import { act, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { stubImages } from "../test-utils";
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

  afterEach(() => vi.unstubAllGlobals());

  it("asks for no picture when there is none, and shows the initial", () => {
    const requested = stubImages();
    render(<UserAvatar name="Mali" />);
    expect(screen.getByText("M")).toBeInTheDocument();
    expect(requested).toHaveLength(0);
  });

  it("keeps the initial until the picture loads, then shows the picture", () => {
    const requested = stubImages();
    const { container } = render(<UserAvatar name="Mali" src="https://cdn.example.com/m.png" />);
    expect(screen.getByText("M")).toBeInTheDocument();
    act(() => requested[0]!.settle(true));
    expect(container.querySelector("img")).toHaveAttribute("src", "https://cdn.example.com/m.png");
    expect(screen.queryByText("M")).not.toBeInTheDocument();
  });

  it("shows the initial when the picture fails to load", () => {
    const requested = stubImages();
    const { container } = render(<UserAvatar name="Mali" src="https://cdn.example.com/broken.png" />);
    act(() => requested[0]!.settle(false));
    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText("M")).toBeInTheDocument();
  });
});
