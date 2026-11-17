import type { ApprovalStatus } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
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
}

/**
 * Approve, reject with a reason, or cancel, as the rules allow: only the named approver
 * decides, only the requester cancels, and only while the request is pending (APR-04, APR-05).
 * The server checks the same rules; the buttons just hide what would be refused.
 */
export function ApprovalActions({ requestId, requesterId, approverId, status }: Props) {
  const t = useT();
  const me = useMe().data?.id;
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const [reasonMissing, setReasonMissing] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["approval"] });
  const approve = useMutation({ mutationFn: () => api.approve(requestId), onSuccess: refresh });
  const reject = useMutation({
    mutationFn: (why: string) => api.reject(requestId, why),
    onSuccess: () => {
      setRejecting(false);
      setReason("");
      return refresh();
    },
  });
  const cancel = useMutation({ mutationFn: () => api.cancel(requestId), onSuccess: refresh });

  const pending = status === "pending";
  const canDecide = pending && me === approverId;
  const canCancel = pending && me === requesterId;
  if (!canDecide && !canCancel) return null;

  const busy = approve.isPending || reject.isPending || cancel.isPending;
  const error = approve.error ?? reject.error ?? cancel.error;

  function submitReject(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const why = reason.trim();
    setReasonMissing(why === "");
    if (why === "") return (e.currentTarget.elements.namedItem("reason") as HTMLElement).focus();
    reject.mutate(why);
  }

  return (
    <div className="approval-actions">
      {canDecide && !rejecting && (
        <div className="actions">
          <button type="button" disabled={busy} onClick={() => approve.mutate()}>
            {t("approval.approve")}
          </button>
          <button type="button" className="danger" disabled={busy} onClick={() => setRejecting(true)}>
            {t("approval.reject")}
          </button>
        </div>
      )}
      {canDecide && rejecting && (
        <form onSubmit={submitReject} noValidate>
          <Field label={t("approval.reason")} error={reasonMissing ? t("approval.reasonRequired") : undefined}>
            {({ id, describedBy, invalid }) => (
              <textarea
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
          <div className="actions">
            <button type="submit" className="danger" disabled={busy}>
              {t("approval.rejectConfirm")}
            </button>
            <button
              type="button"
              onClick={() => {
                setRejecting(false);
                setReasonMissing(false);
              }}
            >
              {t("cancel")}
            </button>
          </div>
        </form>
      )}
      {canCancel && (
        <div className="actions">
          <button type="button" disabled={busy} onClick={() => cancel.mutate()}>
            {t("approval.cancelRequest")}
          </button>
        </div>
      )}
      <FormError message={error ? t(errorKey(error, { 409: "approval.alreadyDecided" })) : null} />
    </div>
  );
}
