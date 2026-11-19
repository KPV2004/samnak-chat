import type { ServerEventMap } from "@samnak/core";
import { useEffect } from "react";
import { toast } from "sonner";
import { useT } from "../i18n/useT";
import { useChat } from "../state/chat";
import { useSession } from "../state/session";
import { notificationTitle } from "./notificationText";

// The frame carries `quiet: true` inside do-not-disturb (NOTI-07). Core's event type does not name it yet.
type CreatedFrame = ServerEventMap["notification.created"] & { quiet?: boolean };

/**
 * Pops a toast for each notice that arrives in the workspace on screen, except
 * inside do-not-disturb. The bell's count moves for every notice regardless
 * (the store listens to the same frame).
 */
export function useNotificationToasts(enabled: boolean) {
  const t = useT();
  const session = useSession();
  const realtime = useChat()?.realtime ?? null;

  useEffect(() => {
    if (!enabled || !realtime) return;
    return realtime.on("notification.created", (frame: CreatedFrame) => {
      if (frame.quiet || frame.workspace_id !== session.getState().workspaceId) return;
      toast(notificationTitle(frame.notification, t), { description: frame.notification.body || undefined });
    });
  }, [enabled, realtime, session, t]);
}
