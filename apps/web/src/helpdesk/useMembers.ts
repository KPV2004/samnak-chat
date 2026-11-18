import type { Member } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useCallback } from "react";
import { useT } from "../i18n/useT";
import { useSession, useSessionState } from "../state/session";

/**
 * Every member of the current workspace, read a page at a time: it names the
 * requesters, assignees and history actors of a ticket. The query key matches
 * the one the other modules use, so the list is fetched once.
 */
export function useMembers() {
  const session = useSession();
  const { workspaceId } = useSessionState();
  return useQuery<Member[]>({
    queryKey: ["all-members", workspaceId],
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

/** Resolves a user id to a name: display name, else e-mail, else a placeholder. */
export function useNameOf(): (userId: string) => string {
  const t = useT();
  const { data } = useMembers();
  return useCallback(
    (userId) => {
      const member = data?.find((m) => m.user_id === userId);
      return member?.display_name || member?.email || t("helpdesk.unknownUser");
    },
    [data, t],
  );
}
