import { ApiError } from "@samnak/core";
import { describe, expect, it } from "vitest";
import { translate } from "../i18n/i18n";
import { commandErrorText } from "./commandError";

const en = (key: Parameters<typeof translate>[1], vars?: Record<string, string | number>) => translate("en", key, vars);
const invalid = (message: string) => new ApiError(400, "invalid", message);

describe("commandErrorText", () => {
  it("explains a missing default category in the user's language", () => {
    const err = invalid("no_default_category: open the ticket form to choose a category");
    expect(commandErrorText(err, en)).toBe("No default category yet: set one in Helpdesk settings or use Create ticket");
    expect(commandErrorText(err, (k, v) => translate("th", k, v))).toContain("แจ้งปัญหา");
  });

  it("translates the usage line and keeps the syntax as the server wrote it", () => {
    expect(commandErrorText(invalid("usage: /ticket <title>"), en)).toBe("Usage: /ticket <title>");
  });

  it("falls back to the server's reason for a code it does not know", () => {
    expect(commandErrorText(invalid("title is too long"), en)).toBe("title is too long");
  });

  it("uses the generic messages for other statuses and for network errors", () => {
    expect(commandErrorText(new ApiError(403, "forbidden", "nope"), en)).toBe(en("error.forbidden"));
    expect(commandErrorText(new ApiError(404, "not_found", "x"), en)).toBe(en("error.not_found"));
    expect(commandErrorText(new TypeError("fetch failed"), en)).toBe(en("error.network"));
  });
});
