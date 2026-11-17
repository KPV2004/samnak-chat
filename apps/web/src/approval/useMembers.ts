import type { Member } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useSession, useSessionState } from "../state/session";

/**
 * Every member of the current workspace, read a page at a time. It gives names to the
 * requests, the approver picker and the group editor.
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
