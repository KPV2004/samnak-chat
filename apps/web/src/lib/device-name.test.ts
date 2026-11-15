import { describe, expect, it } from "vitest";
import { deviceName } from "./device-name";

describe("deviceName", () => {
  it.each([
    ["Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/130.0 Safari/537.36", "Chrome (web)"],
    ["Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/130.0 Safari/537.36 Edg/130.0", "Edge (web)"],
    ["Mozilla/5.0 (X11; Linux) Gecko/20100101 Firefox/131.0", "Firefox (web)"],
    ["Mozilla/5.0 (Macintosh) AppleWebKit/605.1.15 Version/17.0 Safari/605.1.15", "Safari (web)"],
    ["something else", "Browser (web)"],
  ])("%s", (ua, expected) => expect(deviceName(ua)).toBe(expected));
});
