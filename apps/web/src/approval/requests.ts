import { useInfiniteQuery } from "@tanstack/react-query";
import { useSessionState } from "../state/session";
import { useApprovalApi } from "./api";

/** requester: my requests; approver: waiting for me (APR-06). */
export type RequestRole = "requester" | "approver";

/** The two views of the Approvals app; `waiting` is the default. */
export type ApprovalView = "waiting" | "mine";

export const viewRole: Record<ApprovalView, RequestRole> = { waiting: "approver", mine: "requester" };

export function approvalViewOf(raw: string | null): ApprovalView {
  return raw === "mine" ? "mine" : "waiting";
}

export function approvalPath(view: ApprovalView): string {
  return view === "mine" ? "/approvals?view=mine" : "/approvals";
}

/**
 * One list of approval requests, read a page at a time. The list and the sidebar's count share the
 * query key, so the count is the list already loaded; `enabled: false` only reads what is cached.
 */
export function useApprovalRequests(role: RequestRole, enabled = true) {
  const api = useApprovalApi();
  const { workspaceId } = useSessionState();
  return useInfiniteQuery({
    queryKey: ["approval", workspaceId, "requests", role],
    initialPageParam: "",
    // "Waiting for me" holds only what still needs a decision; decided ones stay in the chat cards.
    queryFn: ({ pageParam }) =>
      api.listRequests({ role, status: role === "approver" ? "pending" : undefined, cursor: pageParam || undefined, limit: 20 }),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: enabled && workspaceId !== null,
  });
}

/** How many requests wait for me, from the list as far as it is loaded, and whether more may follow. */
export function useWaitingCount(enabled = true): { n: number; more: boolean } {
  const waiting = useApprovalRequests("approver", enabled);
  return { n: waiting.data?.pages.reduce((n, p) => n + p.items.length, 0) ?? 0, more: waiting.hasNextPage };
}
