import type { Channel, Member } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useCallback } from "react";
import { useT, type T } from "../i18n/useT";
import { useMe } from "../state/queries";
import { useSession, useSessionState } from "../state/session";

/** Every member of the workspace, who gives the senders their names. */
export function useWorkspaceMembers() {
  const session = useSession();
  const { workspaceId } = useSessionState();
  return useQuery<Member[]>({
    queryKey: ["chat-members", workspaceId],
    enabled: workspaceId !== null,
    staleTime: 60_000,
    queryFn: async () => {
      const all: Member[] = [];
      let cursor = "";
      do {
        const page = await session.api.listMembers({ cursor, limit: 100 });
        all.push(...page.items);
        cursor = page.next_cursor;
      } while (cursor);
      return all;
    },
  });
}

/** Resolves a user id to a display name: the member's name, else the e-mail, else a placeholder. */
export function useNameOf(): (userId: string) => string {
  const t = useT();
  const { data } = useWorkspaceMembers();
  return useCallback(
    (userId) => {
      const member = data?.find((m) => m.user_id === userId);
      return member?.display_name || member?.email || t("chat.unknownUser");
    },
    [data, t],
  );
}

/** `general` for a channel, the other people's names for a direct message. */
export function channelTitle(channel: Channel, me: string, nameOf: (userId: string) => string, t: T): string {
  if (channel.type !== "dm") return channel.name;
  const others = channel.dm_user_ids.filter((id) => id !== me);
  const names = (others.length > 0 ? others : [me]).map(nameOf);
  return names.join(", ") || t("chat.dms");
}

/**
 * The picture of a user, or an empty string. Only the signed-in user's own `avatar_url` is known:
 * the member lists the server sends (`Member`, `ChannelMember`) carry no avatar, so everyone else
 * shows initials until it does.
 */
export function useAvatarOf(): (userId: string) => string {
  const me = useMe().data;
  return useCallback((userId) => (me && me.id === userId ? me.avatar_url : ""), [me]);
}
