import { createM3Api, type NotifyPreferences } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useModuleEnabled } from "../modules";
import { useSession, useSessionState } from "../state/session";

/** My notification settings in this workspace (NOTI-07). Asked for only while notify is on. */
export function usePreferences() {
  const session = useSession();
  const { workspaceId } = useSessionState();
  const enabled = useModuleEnabled("notify");
  return useQuery({
    queryKey: ["notify-preferences", workspaceId],
    queryFn: () => createM3Api(session.client).getPreferences(),
    enabled: enabled && workspaceId !== null,
  });
}

/** Saves the whole settings object, and keeps the cached copy in step with what the server kept. */
export function useSavePreferences() {
  const session = useSession();
  const { workspaceId } = useSessionState();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (prefs: NotifyPreferences) => createM3Api(session.client).setPreferences(prefs),
    onSuccess: (saved) => queryClient.setQueryData(["notify-preferences", workspaceId], saved),
  });
}

/**
 * Whether a channel is muted for me, and a toggle for it (NOTI-07). Nothing can
 * be toggled until the settings have loaded, so a toggle never writes stale ones.
 */
export function useChannelMute(channelId: string) {
  const t = useT();
  const prefs = usePreferences();
  const save = useSavePreferences();
  const current = prefs.data;
  const muted = current?.muted_channel_ids.includes(channelId) ?? false;

  function toggle() {
    if (!current) return;
    const muted_channel_ids = muted
      ? current.muted_channel_ids.filter((id) => id !== channelId)
      : [...current.muted_channel_ids, channelId];
    save.mutate(
      { ...current, muted_channel_ids },
      {
        onSuccess: () => toast.success(t(muted ? "notify.unmuted" : "notify.muted")),
        onError: (err) => toast.error(t(errorKey(err))),
      },
    );
  }

  return { ready: current !== undefined, muted, pending: save.isPending, toggle };
}
