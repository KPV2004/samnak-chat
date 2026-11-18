import type { ApprovalType, CreateApprovalInput } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";
import { useMe } from "../state/queries";
import { useApprovalApi } from "./api";
import { leaveTypeKeys } from "./format";
import { useMembers } from "./useMembers";

const maxFileBytes = 50 * 1024 * 1024;
const amountPattern = /^\d+(\.\d+)?$/;
const leaveTypes = ["annual", "sick", "personal", "other"] as const;

interface Attached {
  id: string;
  name: string;
}

type Errors = Partial<Record<string, MessageKey>>;

/** The checks the form can make before the server does. Returns the message key of each bad field. */
function validate(type: ApprovalType, v: Record<string, string>): Errors {
  const errors: Errors = {};
  const required = (name: string) => {
    if (!v[name]?.trim()) errors[name] = "approval.required";
  };
  if (type === "leave") {
    required("start_date");
    required("end_date");
    required("reason");
    if (v.start_date && v.end_date && v.end_date < v.start_date) errors.end_date = "approval.datesOrder";
    if (v.half_day && v.half_day !== "none" && v.start_date && v.end_date && v.start_date !== v.end_date) {
      errors.half_day = "approval.halfDayOneDay";
    }
  } else {
    required("subject");
    if (v.amount?.trim() && !amountPattern.test(v.amount.trim())) errors.amount = "approval.amountInvalid";
  }
  return errors;
}

/** The body of `POST /approval/requests`: no approver when the default one is kept, no half day unless set. */
function buildInput(type: ApprovalType, v: Record<string, string>, attachmentIds: string[]): CreateApprovalInput {
  const text = (name: string) => (v[name] ?? "").trim();
  const payload: Record<string, unknown> =
    type === "leave"
      ? {
          leave_type: v.leave_type,
          start_date: v.start_date,
          end_date: v.end_date,
          reason: text("reason"),
          ...(text("half_day") && text("half_day") !== "none" ? { half_day: text("half_day") } : {}),
        }
      : {
          subject: text("subject"),
          details: text("details"),
          // The server takes a currency only together with an amount.
          ...(text("amount") ? { amount: text("amount"), currency: text("currency") || "THB" } : {}),
        };
  return {
    type,
    payload,
    attachment_ids: attachmentIds,
    ...(v.approver_id ? { approver_id: v.approver_id } : {}),
  };
}

interface Props {
  /** Closes the dialog that holds the form: after a sent request, or on Cancel. */
  onDone: () => void;
}

/** The form that sends a new leave or general request (APR-01, APR-02, APR-09). */
export function NewRequestForm({ onDone }: Props) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const me = useMe().data?.id;
  const members = useMembers();
  const [type, setType] = useState<ApprovalType>("leave");
  const [errors, setErrors] = useState<Errors>({});
  const [files, setFiles] = useState<Attached[]>([]);
  const [uploading, setUploading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: (input: CreateApprovalInput) => api.createRequest(input),
    onSuccess: () => {
      toast.success(t("approval.toast.sent"));
      return queryClient.invalidateQueries({ queryKey: ["approval"] });
    },
  });

  // Each file is uploaded straight to storage first; the request then names the ids (APR-09).
  async function addFiles(picked: File[]) {
    setFileError(null);
    setUploading(true);
    try {
      for (const file of picked) {
        if (file.size > maxFileBytes) {
          setFileError(t("approval.fileTooLarge", { name: file.name }));
          continue;
        }
        const upload = await api.requestUpload({
          file_name: file.name,
          mime_type: file.type || "application/octet-stream",
          size: file.size,
        });
        const sent = await fetch(upload.upload.url, { method: upload.upload.method, headers: upload.upload.headers, body: file });
        if (!sent.ok) {
          setFileError(t("error.generic"));
          return;
        }
        setFiles((prev) => [...prev, { id: upload.attachment_id, name: file.name }]);
      }
    } catch (err) {
      setFileError(t(errorKey(err)));
    } finally {
      setUploading(false);
    }
  }

  function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const v = readForm(form);
    const found = validate(type, v);
    setErrors(found);
    const first = Object.keys(found)[0];
    if (first) return focusField(form, first);
    create.mutate(buildInput(type, v, files.map((f) => f.id)), {
      onSuccess: () => {
        form.reset();
        setType("leave");
        setFiles([]);
        onDone();
      },
    });
  }

  const message = (name: string) => {
    const key = errors[name];
    return key ? t(key) : undefined;
  };
  const approvers = (members.data ?? []).filter((m) => m.user_id !== me && m.status === "active");
  const error = fileError ?? (create.error ? t(errorKey(create.error)) : null);

  return (
    <form onSubmit={onSubmit} noValidate>
      <Field label={t("approval.type")}>
        {({ id, describedBy }) => (
          <NativeSelect
            id={id}
            aria-describedby={describedBy}
            containerClassName="w-full"
            value={type}
            onChange={(e) => {
              setType(e.target.value as ApprovalType);
              setErrors({});
            }}
          >
            <option value="leave">{t("approval.type.leave")}</option>
            <option value="general">{t("approval.type.general")}</option>
          </NativeSelect>
        )}
      </Field>
      <Field label={t("approval.approver")}>
        {({ id, describedBy }) => (
          <NativeSelect id={id} name="approver_id" defaultValue="" aria-describedby={describedBy} containerClassName="w-full">
            <option value="">{t("approval.approverDefault")}</option>
            {approvers.map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.display_name || m.email}
              </option>
            ))}
          </NativeSelect>
        )}
      </Field>

      {type === "leave" ? (
        <>
          <Field label={t("approval.leaveType")}>
            {({ id, describedBy }) => (
              <NativeSelect id={id} name="leave_type" defaultValue="annual" aria-describedby={describedBy} containerClassName="w-full">
                {leaveTypes.map((k) => (
                  <option key={k} value={k}>
                    {t(leaveTypeKeys[k])}
                  </option>
                ))}
              </NativeSelect>
            )}
          </Field>
          <Field label={t("approval.startDate")} name="start_date" type="date" error={message("start_date")} />
          <Field label={t("approval.endDate")} name="end_date" type="date" error={message("end_date")} />
          <Field label={t("approval.halfDay")} hint={t("approval.halfDayHint")} error={message("half_day")}>
            {({ id, describedBy, invalid }) => (
              <NativeSelect
                id={id}
                name="half_day"
                defaultValue="none"
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
                containerClassName="w-full"
              >
                <option value="none">{t("approval.halfDay.none")}</option>
                <option value="morning">{t("approval.halfDay.morning")}</option>
                <option value="afternoon">{t("approval.halfDay.afternoon")}</option>
              </NativeSelect>
            )}
          </Field>
          <Field label={t("approval.reason")} error={message("reason")}>
            {({ id, describedBy, invalid }) => (
              <Textarea
                id={id}
                name="reason"
                rows={3}
                maxLength={1000}
                aria-describedby={describedBy}
                aria-invalid={invalid || undefined}
              />
            )}
          </Field>
        </>
      ) : (
        <>
          <Field label={t("approval.subject")} name="subject" maxLength={200} error={message("subject")} />
          <Field label={t("approval.details")}>
            {({ id, describedBy }) => <Textarea id={id} name="details" rows={4} maxLength={4000} aria-describedby={describedBy} />}
          </Field>
          <Field label={t("approval.amount")} name="amount" inputMode="decimal" hint={t("approval.amountHint")} error={message("amount")} />
          <Field label={t("approval.currency")} name="currency" defaultValue="THB" maxLength={3} />
        </>
      )}

      <Field
        label={t("approval.attachments")}
        type="file"
        multiple
        hint={t("approval.attachmentsHint")}
        onChange={(e) => {
          const picked = Array.from(e.target.files ?? []);
          e.target.value = "";
          void addFiles(picked);
        }}
      />
      {uploading && (
        <p role="status" className="m-0 mb-4 text-sm text-muted-foreground">
          {t("approval.uploading")}
        </p>
      )}
      {files.length > 0 && (
        <ul aria-label={t("approval.attachments")} className="m-0 mb-4 grid list-none gap-1 p-0">
          {files.map((f) => (
            <li key={f.id} className="flex items-center justify-between gap-2 rounded-md border px-3 py-1.5 text-sm">
              <span className="min-w-0 break-all">{f.name}</span>
              <Button
                type="button"
                variant="ghost"
                size="sm"
                aria-label={t("approval.removeFile", { name: f.name })}
                onClick={() => setFiles((prev) => prev.filter((x) => x.id !== f.id))}
              >
                {t("members.remove")}
              </Button>
            </li>
          ))}
        </ul>
      )}

      <FormError message={error} />
      <div className="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button type="button" variant="outline" onClick={onDone}>
          {t("cancel")}
        </Button>
        <Button type="submit" disabled={create.isPending || uploading}>
          {t("approval.submit")}
        </Button>
      </div>
    </form>
  );
}
