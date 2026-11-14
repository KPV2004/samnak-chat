import { describe, expect, it } from "vitest";
import { messages } from "./messages";
import { detectLocale, translate } from "./i18n";

describe("i18n", () => {
  it("has the same keys in Thai and English", () => {
    expect(Object.keys(messages.th).sort()).toEqual(Object.keys(messages.en).sort());
  });
  it("translates by locale", () => {
    expect(translate("en", "loading")).toBe("Loading…");
    expect(translate("th", "loading")).toBe("กำลังโหลด…");
  });
  it("detects locale, defaulting to Thai", () => {
    expect(detectLocale("en-US")).toBe("en");
    expect(detectLocale("th-TH")).toBe("th");
    expect(detectLocale(undefined)).toBe("th");
  });
});
