import { ApiError } from "@samnak/core";
import type { T } from "../i18n/useT";
import { errorKey } from "../lib/errors";

/**
 * Why a slash command failed, as the sender should read it. The server answers 400 `invalid` with
 * its reason in the message (`no_default_category: …`, `usage: /ticket <title>`); the reasons we
 * know are translated, any other reason is shown as the server wrote it. Other failures use the
 * generic messages.
 */
export function commandErrorText(err: unknown, t: T): string {
  if (!(err instanceof ApiError) || err.status !== 400) return t(errorKey(err));
  const message = err.message.trim();
  if (message.startsWith("no_default_category")) return t("chat.command.noDefaultCategory");
  const usage = message.match(/^usage:\s*(.+)$/i);
  if (usage) return t("chat.command.usage", { usage: usage[1]! });
  return message || t(errorKey(err));
}
