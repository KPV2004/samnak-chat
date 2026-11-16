import { createM3Api, createNotificationStore, type NotificationState } from "@samnak/core";
import { useEffect, useMemo, useSyncExternalStore } from "react";
import { useChat } from "../state/chat";
import { useSession, useSessionState } from "../state/session";

const empty: NotificationState = { unreadCount: 0, items: [], hasMore: false, loaded: false, status: "idle" };
const never = () => () => {};

/**
 * Builds the notification store on top of the chat runtime's realtime client
 * (one socket per session, not a second one). Nothing is created while notify
 * is disabled or the user is not signed in, so a notify-less server sees no
 * `/notify` traffic at all.
 */
export function useNotificationStore(enabled: boolean) {
  const session = useSession();
  const { workspaceId } = useSessionState();
  const realtime = useChat()?.realtime ?? null;

  const store = useMemo(
    () =>
      enabled && realtime
        ? createNotificationStore({
            api: createM3Api(session.client),
            realtime,
            getWorkspaceId: () => session.getState().workspaceId,
          })
        : null,
    [enabled, realtime, session],
  );

  useEffect(() => () => store?.dispose(), [store]);

  // Notifications belong to a workspace: drop the last one's and count this one's.
  useEffect(() => {
    if (!store) return;
    store.reset();
    if (workspaceId) void store.loadCount();
  }, [store, workspaceId]);

  const state = useSyncExternalStore(store?.subscribe ?? never, store?.getState ?? (() => empty));
  return { store, state };
}
