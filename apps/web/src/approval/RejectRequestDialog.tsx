import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useApprovalApi } from "./api";

interface Props {
  /** The request to refuse; `null` keeps the dialog closed. */
  requestId: string | null;
  onClose: () => void;
}

/**
 * The reject dialog, opened from a detail page, a list row or a Home card (AC-40, APR-05).
 * The refusal needs a reason before it can be sent, and the list of the same key refreshes after.
 */
export function RejectRequestDialog({ requestId, onClose }: Props) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const [reason, setReason] = useState("");
  const [reasonMissing, setReasonMissing] = useState(false);

  const reject = useMutation({
    mutationFn: (why: string) => api.reject(requestId ?? "", why),
    onSuccess: () => {
      toast.success(t("approval.toast.rejected"));
      close();
      return queryClient.invalidateQueries({ queryKey: ["approval"] });
    },
  });

  function close() {
    onClose();
    setReason("");
    setReasonMissing(false);
    reject.reset();
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const why = reason.trim();
    setReasonMissing(why === "");
    if (why === "") return (e.currentTarget.elements.namedItem("reason") as HTMLElement).focus();
    reject.mutate(why);
  }

  return (
    <Dialog open={requestId !== null} onOpenChange={(open) => !open && close()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("approval.rejectTitle")}</DialogTitle>
          <DialogDescription>{t("approval.rejectHint")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={submit} noValidate>
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
            <Button type="button" variant="outline" onClick={close}>
              {t("cancel")}
            </Button>
            <Button type="submit" variant="destructive" disabled={reject.isPending}>
              {t("approval.rejectConfirm")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
