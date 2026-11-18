import type { HelpdeskQueue, TicketPriority } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { NativeSelect } from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useHelpdeskApi, useHelpdeskFileApi, uploadAttachment } from "./api";
import { priorityKeys, priorities } from "./format";

const maxDescription = 8000;
const maxFileBytes = 50 * 1024 * 1024;

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Every queue with the categories that take tickets. */
  queues: HelpdeskQueue[];
  /** Text the dialog starts with, e.g. the message a ticket is made from (M5-05). */
  initialDescription?: string;
  sourceMessageId?: string;
}

/**
 * The new-ticket form (HD-03): a category, a title, optional details, a priority and files. It is a
 * dialog so the list stays in view, and it can be opened from a message with the message's text.
 */
export function NewTicketDialog({ open, onOpenChange, queues, initialDescription = "", sourceMessageId }: Props) {
  const t = useT();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("helpdesk.new.title")}</DialogTitle>
          <DialogDescription>{t("helpdesk.new.hint")}</DialogDescription>
        </DialogHeader>
        <NewTicketForm
          queues={queues}
          initialDescription={initialDescription}
          sourceMessageId={sourceMessageId}
          onDone={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}

interface Attached {
  id: string;
  name: string;
}

/**
 * The form itself. It is mounted only while the dialog is open, so every opening starts clean: the
 * default category, no files.
 */
function NewTicketForm({
  queues,
  initialDescription,
  sourceMessageId,
  onDone,
}: Pick<Props, "queues" | "initialDescription" | "sourceMessageId"> & { onDone: () => void }) {
  const t = useT();
  const api = useHelpdeskApi();
  const fileApi = useHelpdeskFileApi();
  const queryClient = useQueryClient();
  const categories = queues.flatMap((q) => q.categories.map((c) => ({ ...c, queueName: q.name })));
  const fallback = categories.find((c) => c.is_default) ?? categories[0];
  const [categoryId, setCategoryId] = useState(fallback?.id ?? "");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState(initialDescription ?? "");
  const [priority, setPriority] = useState<TicketPriority>("normal");
  const [titleError, setTitleError] = useState(false);
  const [descriptionError, setDescriptionError] = useState(false);
  const [files, setFiles] = useState<Attached[]>([]);
  const [uploading, setUploading] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const create = useMutation({
    mutationFn: () => {
      const trimmed = description.trim();
      return api.createTicket({
        category_id: category,
        title: title.trim(),
        ...(trimmed ? { description: trimmed } : {}),
        priority,
        ...(sourceMessageId ? { source_message_id: sourceMessageId } : {}),
        ...(files.length > 0 ? { attachment_ids: files.map((f) => f.id) } : {}),
      });
    },
    onSuccess: (ticket) => {
      toast.success(t("helpdesk.new.created", { key: ticket.key }));
      void queryClient.invalidateQueries({ queryKey: ["helpdesk", "tickets"] });
      onDone();
    },
  });

  // The fallback arrives with the queues, which may load after this mounts.
  const category = categoryId || fallback?.id || "";
  const noCategories = categories.length === 0;

  // Each file goes straight to storage first; the ticket then names the ids (HD-03).
  async function addFiles(picked: File[]) {
    setFileError(null);
    setUploading(true);
    try {
      for (const file of picked) {
        if (file.size > maxFileBytes) {
          setFileError(t("helpdesk.fileTooLarge", { name: file.name }));
          continue;
        }
        const id = await uploadAttachment(fileApi, file);
        if (id === null) {
          setFileError(t("error.generic"));
          return;
        }
        setFiles((prev) => [...prev, { id, name: file.name }]);
      }
    } catch (err) {
      setFileError(t(errorKey(err)));
    } finally {
      setUploading(false);
    }
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const missing = title.trim() === "";
    const tooLong = description.trim().length > maxDescription;
    setTitleError(missing);
    setDescriptionError(tooLong);
    if (missing) {
      (e.currentTarget.elements.namedItem("ticket-title") as HTMLElement | null)?.focus();
      return;
    }
    if (tooLong) return;
    create.mutate();
  }

  const error = fileError ?? (create.error ? t(errorKey(create.error)) : null);

  return (
    <>
      {noCategories ? <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.new.noCategories")}</p> : null}
      <form onSubmit={submit} noValidate className="grid gap-4">
        {!noCategories && (
          <>
            <Field label={t("helpdesk.category")}>
              {({ id, describedBy }) => (
                <NativeSelect
                  id={id}
                  aria-describedby={describedBy}
                  containerClassName="w-full"
                  value={category}
                  onChange={(e) => setCategoryId(e.target.value)}
                >
                  {categories.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.queueName} · {c.name}
                    </option>
                  ))}
                </NativeSelect>
              )}
            </Field>
            <Field
              label={t("helpdesk.field.title")}
              hint={t("helpdesk.field.titleHint")}
              error={titleError ? t("helpdesk.field.titleRequired") : undefined}
            >
              {({ id, describedBy, invalid }) => (
                <input
                  id={id}
                  name="ticket-title"
                  className="h-9 w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none selection:bg-primary focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive"
                  maxLength={200}
                  value={title}
                  aria-invalid={invalid || undefined}
                  aria-describedby={describedBy}
                  onChange={(e) => setTitle(e.target.value)}
                />
              )}
            </Field>
            <Field
              label={t("helpdesk.field.description")}
              hint={t("helpdesk.field.descriptionHint")}
              error={descriptionError ? t("helpdesk.field.descriptionTooLong") : undefined}
            >
              {({ id, describedBy, invalid }) => (
                <Textarea
                  id={id}
                  rows={4}
                  value={description}
                  aria-invalid={invalid || undefined}
                  aria-describedby={describedBy}
                  onChange={(e) => setDescription(e.target.value)}
                />
              )}
            </Field>
            <Field label={t("helpdesk.priority")}>
              {({ id, describedBy }) => (
                <NativeSelect
                  id={id}
                  aria-describedby={describedBy}
                  containerClassName="w-full"
                  value={priority}
                  onChange={(e) => setPriority(e.target.value as TicketPriority)}
                >
                  {priorities.map((p) => (
                    <option key={p} value={p}>
                      {t(priorityKeys[p])}
                    </option>
                  ))}
                </NativeSelect>
              )}
            </Field>
            <Field
              label={t("helpdesk.attachments")}
              type="file"
              multiple
              hint={t("helpdesk.attachmentsHint")}
              onChange={(e) => {
                const picked = Array.from(e.target.files ?? []);
                e.target.value = "";
                void addFiles(picked);
              }}
            />
            {uploading && (
              <p role="status" className="m-0 text-sm text-muted-foreground">
                {t("helpdesk.uploading")}
              </p>
            )}
            {files.length > 0 && (
              <ul aria-label={t("helpdesk.attachedFiles")} className="m-0 grid list-none gap-1 p-0">
                {files.map((f) => (
                  <li key={f.id} className="flex items-center justify-between gap-2 rounded-md border px-3 py-1.5 text-sm">
                    <span className="min-w-0 break-all">{f.name}</span>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      aria-label={t("helpdesk.removeFile", { name: f.name })}
                      onClick={() => setFiles((prev) => prev.filter((x) => x.id !== f.id))}
                    >
                      {t("members.remove")}
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
        <FormError message={error} />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onDone}>
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={create.isPending || uploading || noCategories}>
            {t("helpdesk.new.submit")}
          </Button>
        </DialogFooter>
      </form>
    </>
  );
}
