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
import { useHelpdeskApi } from "./api";
import { priorityKeys, priorities } from "./format";

const maxDescription = 8000;

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
 * The new-ticket form (HD-03): a category, a title, optional details and a
 * priority. It is a dialog so the list stays in view, and it can be opened
 * from a message later with the message's text.
 */
export function NewTicketDialog({ open, onOpenChange, queues, initialDescription = "", sourceMessageId }: Props) {
  const t = useT();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const categories = queues.flatMap((q) => q.categories.map((c) => ({ ...c, queueName: q.name })));
  const fallback = categories.find((c) => c.is_default) ?? categories[0];
  // The dialog unmounts its content when it closes, so every opening starts
  // clean, at the default category.
  const [categoryId, setCategoryId] = useState(fallback?.id ?? "");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState(initialDescription);
  const [priority, setPriority] = useState<TicketPriority>("normal");
  const [titleError, setTitleError] = useState(false);
  const [descriptionError, setDescriptionError] = useState(false);

  const create = useMutation({
    mutationFn: () => {
      const trimmed = description.trim();
      return api.createTicket({
        category_id: category,
        title: title.trim(),
        ...(trimmed ? { description: trimmed } : {}),
        priority,
        ...(sourceMessageId ? { source_message_id: sourceMessageId } : {}),
      });
    },
    onSuccess: (ticket) => {
      toast.success(t("helpdesk.new.created", { key: ticket.key }));
      void queryClient.invalidateQueries({ queryKey: ["helpdesk", "tickets"] });
      onOpenChange(false);
    },
  });

  // The fallback arrives with the queues, which may load after this mounts.
  const category = categoryId || fallback?.id || "";
  const noCategories = categories.length === 0;

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

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("helpdesk.new.title")}</DialogTitle>
          <DialogDescription>{t("helpdesk.new.hint")}</DialogDescription>
        </DialogHeader>
        {noCategories ? (
          <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.new.noCategories")}</p>
        ) : null}
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
          </>
          )}
          <FormError message={create.error ? t(errorKey(create.error)) : null} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending || noCategories}>
              {t("helpdesk.new.submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
