import { ApiError } from "@samnak/core";
import type { MessageKey } from "../i18n/messages";

const byCode: Record<string, MessageKey> = {
  invalid: "error.invalid",
  unauthorized: "error.unauthorized",
  forbidden: "error.forbidden",
  not_found: "error.not_found",
  conflict: "error.conflict",
  rate_limited: "error.rate_limited",
};

/**
 * Maps a failed call to a translated message key; the server's English text is never shown.
 * `byStatus` lets a form say what a status means in its own context (401 on login, 404 on accepting an invite).
 */
export function errorKey(err: unknown, byStatus: Partial<Record<number, MessageKey>> = {}): MessageKey {
  if (err instanceof ApiError) return byStatus[err.status] ?? byCode[err.code] ?? "error.generic";
  return "error.network";
}
