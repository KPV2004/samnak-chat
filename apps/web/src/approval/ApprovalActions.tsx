import type { ApprovalStatus } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useMe } from "../state/queries";
import { useApprovalApi } from "./api";

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
  const [reason, setReason] = useState("");
  const [reasonMissing, setReasonMissing] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["approval"] });
  const approve = useMutation({
    mutationFn: () => api.approve(requestId),
    onSuccess: () => {
      toast.success(t("approval.toast.approved"));
      return refresh();
    },
  });
  const reject = useMutation({
    mutationFn: (why: string) => api.reject(requestId, why),
    onSuccess: () => {
      toast.success(t("approval.toast.rejected"));
      setRejecting(false);
      setReason("");
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

  const busy = approve.isPending || reject.isPending || cancel.isPending;
  const error = approve.error ?? cancel.error;

  function closeReject(open: boolean) {
    setRejecting(open);
    if (!open) {
      setReason("");
      setReasonMissing(false);
      reject.reset();
    }
  }

  function submitReject(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const why = reason.trim();
    setReasonMissing(why === "");
    if (why === "") return (e.currentTarget.elements.namedItem("reason") as HTMLElement).focus();
    reject.mutate(why);
  }

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
      {canDecide && (
        <Dialog open={rejecting} onOpenChange={closeReject}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t("approval.rejectTitle")}</DialogTitle>
              <DialogDescription>{t("approval.rejectHint")}</DialogDescription>
            </DialogHeader>
            <form onSubmit={submitReject} noValidate>
              <Field label={t("approval.reason")} error={reasonMissing ? t("approval.reasonRequired") : undefined}>
                {({ id, describedBy, invalid }) => (
                  <Textarea
                    id={id}
                    name="reason"
                    rows={3}
                    maxLength={1000}
                    value={reason}
                    aria-invalid={invalid || undefined}
                    aria-describedby={describedBy}
                    onChange={(e) => setReason(e.target.value)}
                  />
                )}
              </Field>
              <FormError message={reject.error ? t(errorKey(reject.error, { 409: "approval.alreadyDecided" })) : null} />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => closeReject(false)}>
                  {t("cancel")}
                </Button>
                <Button type="submit" variant="destructive" disabled={reject.isPending}>
                  {t("approval.rejectConfirm")}
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}
