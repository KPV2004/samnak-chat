import type { ApprovalStatus } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useMe } from "../state/queries";
import { useApprovalApi } from "./api";
import { RejectRequestDialog } from "./RejectRequestDialog";

interface Props {
  requestId: string;
  requesterId: string;
  approverId: string;
  status: ApprovalStatus;
  /** `sm` on a chat card, where the buttons sit beside a line of text. */
  size?: "sm" | "default";
}

/**
 * Approve, reject with a reason, or cancel, as the rules allow: only the named approver
 * decides, only the requester cancels, and only while the request is pending (APR-04, APR-05).
 * The server checks the same rules; the buttons just hide what would be refused.
 * Rejecting asks for the reason in a dialog, so the refusal cannot go out by accident.
 */
export function ApprovalActions({ requestId, requesterId, approverId, status, size = "default" }: Props) {
  const t = useT();
  const me = useMe().data?.id;
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const [rejecting, setRejecting] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["approval"] });
  const approve = useMutation({
    mutationFn: () => api.approve(requestId),
    onSuccess: () => {
      toast.success(t("approval.toast.approved"));
      return refresh();
    },
  });
  const cancel = useMutation({
    mutationFn: () => api.cancel(requestId),
    onSuccess: () => {
      toast.success(t("approval.toast.cancelled"));
      return refresh();
    },
  });

  const pending = status === "pending";
  const canDecide = pending && me === approverId;
  const canCancel = pending && me === requesterId;
  if (!canDecide && !canCancel) return null;

  const busy = approve.isPending || cancel.isPending;
  const error = approve.error ?? cancel.error;

  return (
    <div className="grid gap-2">
      {(canDecide || canCancel) && (
        <div className="flex flex-wrap gap-2">
          {canDecide && (
            <Button size={size} disabled={busy} onClick={() => approve.mutate()}>
              {t("approval.approve")}
            </Button>
          )}
          {canDecide && (
            <Button size={size} variant="outline" disabled={busy} onClick={() => setRejecting(true)}>
              {t("approval.reject")}
            </Button>
          )}
          {canCancel && (
            <Button size={size} variant="outline" disabled={busy} onClick={() => cancel.mutate()}>
              {t("approval.cancelRequest")}
            </Button>
          )}
        </div>
      )}
      <FormError message={error ? t(errorKey(error, { 409: "approval.alreadyDecided" })) : null} />
      {canDecide && <RejectRequestDialog requestId={rejecting ? requestId : null} onClose={() => setRejecting(false)} />}
    </div>
  );
}
