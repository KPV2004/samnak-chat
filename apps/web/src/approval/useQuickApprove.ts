import type { ApprovalPage, ApprovalRequest, ApprovalStatus } from "@samnak/core";
import { useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { useCallback } from "react";
import { toast } from "sonner";
import { errorKey } from "../lib/errors";
import { useT } from "../i18n/useT";
import { useSessionState } from "../state/session";
import { useApprovalApi } from "./api";

const graceMs = 5_000;

interface Pending {
  timer: ReturnType<typeof setTimeout>;
  /** Sends now (the grace ended or the toast closed). Runs once. */
  send: () => void;
  /** Stops the send and puts the row back. Runs once. */
  cancel: () => void;
}

/**
 * Timers live outside React: navigating away while the 5-second grace runs must not cancel the
 * decision the user already made. `Undo` stops a request that has not been sent yet; there is no
 * server-side un-approve, so a sent decision is final.
 */
const pending = new Map<string, Pending>();

type ListData = InfiniteData<ApprovalPage, string>;

function withStatus(data: ListData | undefined, id: string, status: ApprovalStatus): ListData | undefined {
  if (!data) return data;
  return {
    ...data,
    pages: data.pages.map((page) => ({
      ...page,
      items: page.items.map((item) => (item.id === id ? { ...item, status } : item)),
    })),
  };
}

/**
 * Quick approve from a list or the Home card: show the decision at once, send it after a
 * 5-second grace, and let Undo cancel the send. The row shares the list's query key, so the list
 * and the card never disagree.
 */
export function useQuickApprove() {
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const { workspaceId } = useSessionState();
  const t = useT();

  return useCallback(
    (request: ApprovalRequest) => {
      if (pending.has(request.id)) return;
      const key = ["approval", workspaceId, "requests", "approver"] as const;
      const patch = (status: ApprovalStatus) =>
        queryClient.setQueryData<ListData>(key, (data) => withStatus(data, request.id, status));

      let finished = false;
      const entry: Pending = {
        timer: setTimeout(() => entry.send(), graceMs),
        send: () => {
          if (finished) return;
          finished = true;
          pending.delete(request.id);
          api.approve(request.id).then(
            () => queryClient.invalidateQueries({ queryKey: ["approval"] }),
            (error: unknown) => {
              patch(request.status);
              const conflict = (error as { status?: number } | null)?.status === 409;
              toast.error(t(errorKey(error, { 409: "approval.alreadyDecided" })));
              if (conflict) void queryClient.invalidateQueries({ queryKey: ["approval"] });
            },
          );
        },
        cancel: () => {
          if (finished) return;
          finished = true;
          clearTimeout(entry.timer);
          pending.delete(request.id);
          patch(request.status);
        },
      };
      pending.set(request.id, entry);
      patch("approved");

      toast(t("approval.quick.sent", { n: graceMs / 1000 }), {
        duration: graceMs,
        action: { label: t("approval.quick.undo"), onClick: entry.cancel },
        // Closing the toast without Undo sends at once, as the spec says.
        onDismiss: entry.send,
      });
    },
    [api, queryClient, workspaceId, t],
  );
}
