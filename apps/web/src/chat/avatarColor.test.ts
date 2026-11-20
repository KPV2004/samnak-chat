import { describe, expect, it } from "vitest";
import { avatarColors, avatarColorFor } from "./avatarColor";

describe("avatarColorFor", () => {
  it("gives the same colour to the same id, every time", () => {
    const id = "0190a8c8-0000-7000-8000-0000000000aa";
    expect(avatarColorFor(id)).toBe(avatarColorFor(id));
    expect(avatarColors).toContain(avatarColorFor(id));
  });

  it("spreads ids over the whole palette", () => {
    const used = new Set(Array.from({ length: 300 }, (_, i) => avatarColorFor(`user-${i}`)));
    expect(used.size).toBe(avatarColors.length);
  });

  it("has no violet: every hue is one the tokens already use", () => {
    const hues = avatarColors.map((c) => Number(/oklch\([\d.]+_[\d.]+_(\d+)\)/.exec(c)?.[1]));
    expect(hues.every((h) => [25, 80, 120, 151, 200].includes(h))).toBe(true);
  });
});
