import type { ChannelState } from "@samnak/core";
import { Badge } from "@/components/ui/badge";
import { useT } from "../i18n/useT";

/** The unread count and the mention count of a conversation, each with its own text for screen readers. */
export function UnreadBadges({ state }: { state: ChannelState }) {
  const t = useT();
  return (
    <>
      {state.unreadCount > 0 && (
        <Badge className="h-5 min-w-5 px-1.5">
          <span aria-hidden="true">{state.unreadCount}</span>
          <span className="sr-only">{t("chat.unread", { n: state.unreadCount })}</span>
        </Badge>
      )}
      {state.mentionCount > 0 && (
        <Badge variant="destructive" className="h-5 min-w-5 px-1.5">
          <span aria-hidden="true">@{state.mentionCount}</span>
          <span className="sr-only">{t("chat.mentions", { n: state.mentionCount })}</span>
        </Badge>
      )}
    </>
  );
}
