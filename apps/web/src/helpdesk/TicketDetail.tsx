import type { HelpdeskQueue, TicketDetail as TicketDetailData, TicketStatus } from "@samnak/core";
import { canAssign, canClaim, canEditTicket, canWriteInternal, statusMoves, type Viewer } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, Paperclip, Send } from "lucide-react";
import { useState, type FormEvent } from "react";
import { Link, useParams } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { historyLine, moveKeys, priorityKeys, priorities, statusKeys, statusTones } from "./format";
import { useNameOf } from "./useMembers";

/** One ticket (HD-05..HD-09, HD-18): its fields, the moves open to the viewer, comments and history. */
export function TicketDetail() {
  return <RequireWorkspace>{() => <TicketView />}</RequireWorkspace>;
}

function TicketView() {
  const t = useT();
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
  const { ticketId = "" } = useParams();
  const ticket = useQuery({
    queryKey: ["helpdesk", "ticket", workspaceId, ticketId],
    queryFn: () => api.getTicket(ticketId),
    enabled: workspaceId !== null && ticketId !== "",
  });
  const queues = useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: workspaceId !== null,
  });

  if (ticket.isPending) {
    return (
      <Page title={t("helpdesk.title")} actions={<BackLink />}>
        <div role="status" className="grid gap-4">
          <span className="sr-only">{t("loading")}</span>
          <Skeleton className="h-40 w-full rounded-xl" />
          <Skeleton className="h-32 w-full rounded-xl" />
        </div>
      </Page>
    );
  }
  if (ticket.isError) {
    return (
      <Page title={t("helpdesk.title")} actions={<BackLink />}>
        <FormError message={t(errorKey(ticket.error, { 404: "helpdesk.notFound" }))} />
      </Page>
    );
  }
  return <Detail ticket={ticket.data} queues={queues.data?.items ?? []} />;
}

function BackLink() {
  const t = useT();
  return (
    <Button asChild variant="outline">
      <Link to="/helpdesk">
        <ArrowLeft />
        {t("helpdesk.back")}
      </Link>
    </Button>
  );
}

function Detail({ ticket, queues }: { ticket: TicketDetailData; queues: HelpdeskQueue[] }) {
  const t = useT();
  const locale = useCurrentLocale();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const me = useMe().data?.id ?? "";
  const workspace = useWorkspaces().current;
  const nameOf = useNameOf();
  const queue = queues.find((q) => q.id === ticket.queue_id);
  const categories = queues.flatMap((q) => q.categories);
  const categoryName = (id: string) => categories.find((c) => c.id === id)?.name ?? "—";

  const viewer: Viewer = {
    userId: me,
    admin: canManage(workspace?.role),
    agentOfQueue: queue?.agent_ids.includes(me) ?? false,
  };
  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["helpdesk", "ticket"] });
    void queryClient.invalidateQueries({ queryKey: ["helpdesk", "tickets"] });
  };
  const tone = statusTones[ticket.status];

  const claim = useMutation({ mutationFn: () => api.assignTicket(ticket.id, me), onSuccess: refresh });
  const assign = useMutation({
    mutationFn: (userId: string) => api.assignTicket(ticket.id, userId),
    onSuccess: refresh,
  });
  const move = useMutation({
    mutationFn: (status: TicketStatus) => api.changeStatus(ticket.id, status),
    onSuccess: refresh,
  });
  const edit = useMutation({
    mutationFn: (patch: { priority?: (typeof priorities)[number]; category_id?: string }) => api.editTicket(ticket.id, patch),
    onSuccess: refresh,
  });
  const moves = statusMoves(ticket, viewer, new Date());
  const actionError = claim.error ?? assign.error ?? move.error ?? edit.error;

  return (
    <Page
      title={
        <span className="inline-flex flex-wrap items-baseline gap-2">
          <span className="font-mono text-base text-muted-foreground">{ticket.key}</span>
          <span>{ticket.title}</span>
        </span>
      }
      description={
        <span className="inline-flex flex-wrap items-center gap-2">
          <Badge variant={tone.badge} className={tone.text}>
            {t(statusKeys[ticket.status])}
          </Badge>
          <span>{t(priorityKeys[ticket.priority])}</span>
          {queue && <span>· {queue.name}</span>}
        </span>
      }
      actions={<BackLink />}
    >
      <Card className="gap-0 py-5">
        <CardContent className="grid gap-5">
          <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
            <Field label={t("helpdesk.requester")} value={nameOf(ticket.requester_id)} />
            <Field label={t("helpdesk.assignee")} value={ticket.assignee_id ? nameOf(ticket.assignee_id) : t("helpdesk.unassigned")} />
            <Field label={t("helpdesk.category")} value={categoryName(ticket.category_id)} />
            <Field label={t("helpdesk.opened")} value={formatDateTime(ticket.created_at, locale)} />
            <Field label={t("helpdesk.updated")} value={formatDateTime(ticket.updated_at, locale)} />
          </dl>
          {ticket.description && (
            <section className="grid gap-1">
              <h2 className="m-0 text-sm font-medium">{t("helpdesk.details")}</h2>
              <p className="m-0 whitespace-pre-wrap text-sm">{ticket.description}</p>
            </section>
          )}
          {ticket.attachments.length > 0 && (
            <section className="grid gap-1">
              <h2 className="m-0 text-sm font-medium">{t("helpdesk.detail.attachments")}</h2>
              <ul className="m-0 grid list-none gap-1 p-0">
                {ticket.attachments.map((a) => (
                  <li key={a.id} className="flex items-center gap-2 text-sm">
                    <Paperclip className="size-4 text-muted-foreground" aria-hidden="true" />
                    {a.file_name}
                  </li>
                ))}
              </ul>
            </section>
          )}
        </CardContent>
      </Card>

      {(canClaim(ticket, viewer) || canAssign(ticket, viewer) || moves.length > 0 || canEditTicket(viewer)) && (
        <Card className="gap-3 py-5">
          <CardHeader>
            <h2 className="text-base font-semibold">{t("helpdesk.detail.moveTo")}</h2>
          </CardHeader>
          <CardContent className="grid gap-3">
            <div className="flex flex-wrap items-end gap-2">
              {canClaim(ticket, viewer) && (
                <Button disabled={claim.isPending} onClick={() => claim.mutate()}>
                  {t("helpdesk.detail.claim")}
                </Button>
              )}
              {moves.map((to) => (
                <Button key={to} variant="outline" disabled={move.isPending} onClick={() => move.mutate(to)}>
                  {t(moveKeys[to])}
                </Button>
              ))}
            </div>
            {canAssign(ticket, viewer) && queue && (
              <label className="grid max-w-sm gap-1 text-sm">
                <span className="font-medium">{t("helpdesk.detail.assignTo")}</span>
                <NativeSelect
                  containerClassName="w-full"
                  value={ticket.assignee_id ?? ""}
                  onChange={(e) => e.target.value && assign.mutate(e.target.value)}
                >
                  <option value="">{t("helpdesk.detail.choose")}</option>
                  {queue.agent_ids.map((id) => (
                    <option key={id} value={id}>
                      {nameOf(id)}
                    </option>
                  ))}
                </NativeSelect>
              </label>
            )}
            {canEditTicket(viewer) && (
              <div className="flex flex-wrap gap-4">
                <label className="grid gap-1 text-sm">
                  <span className="font-medium">{t("helpdesk.priority")}</span>
                  <NativeSelect
                    value={ticket.priority}
                    onChange={(e) => edit.mutate({ priority: e.target.value as (typeof priorities)[number] })}
                  >
                    {priorities.map((p) => (
                      <option key={p} value={p}>
                        {t(priorityKeys[p])}
                      </option>
                    ))}
                  </NativeSelect>
                </label>
                <label className="grid gap-1 text-sm">
                  <span className="font-medium">{t("helpdesk.category")}</span>
                  <NativeSelect
                    value={ticket.category_id}
                    onChange={(e) => e.target.value !== ticket.category_id && edit.mutate({ category_id: e.target.value })}
                  >
                    {categories.map((c) => (
                      <option key={c.id} value={c.id}>
                        {c.name}
                      </option>
                    ))}
                  </NativeSelect>
                </label>
              </div>
            )}
            <FormError
              message={actionError ? t(errorKey(actionError, { 409: "helpdesk.error.invalidTransition", 403: "helpdesk.error.notAllowed" })) : null}
            />
          </CardContent>
        </Card>
      )}

      <CommentSection ticket={ticket} viewer={viewer} />

      <Card className="gap-3 py-5">
        <CardHeader>
          <h2 className="text-base font-semibold">{t("helpdesk.history")}</h2>
        </CardHeader>
        <CardContent>
          {ticket.history.length === 0 ? (
            <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.history.empty")}</p>
          ) : (
            <ol className="m-0 grid list-none gap-2 p-0 text-sm">
              {ticket.history.map((entry) => (
                <li key={entry.id} className="flex flex-wrap items-baseline justify-between gap-2">
                  <span className={entry.internal ? "text-muted-foreground italic" : undefined}>
                    {historyLine(entry, t, { user: nameOf, category: categoryName })}
                  </span>
                  <time dateTime={entry.at} className="text-xs text-muted-foreground">
                    {formatDateTime(entry.at, locale)}
                  </time>
                </li>
              ))}
            </ol>
          )}
        </CardContent>
      </Card>
    </Page>
  );
}

function CommentSection({ ticket, viewer }: { ticket: TicketDetailData; viewer: Viewer }) {
  const t = useT();
  const locale = useCurrentLocale();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const nameOf = useNameOf();
  const [body, setBody] = useState("");
  const [internal, setInternal] = useState(false);
  const [missing, setMissing] = useState(false);
  const comment = useMutation({
    mutationFn: () => api.addComment(ticket.id, { body: body.trim(), internal }),
    onSuccess: () => {
      setBody("");
      setInternal(false);
      setMissing(false);
      void queryClient.invalidateQueries({ queryKey: ["helpdesk", "ticket"] });
    },
  });

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const empty = body.trim() === "";
    setMissing(empty);
    if (empty) return (e.currentTarget.elements.namedItem("comment-body") as HTMLElement | null)?.focus();
    comment.mutate();
  }

  return (
    <Card className="gap-3 py-5">
      <CardHeader>
        <h2 className="text-base font-semibold">{t("helpdesk.comments")}</h2>
      </CardHeader>
      <CardContent className="grid gap-4">
        {ticket.comments.length === 0 ? (
          <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.comment.empty")}</p>
        ) : (
          <ul aria-label={t("helpdesk.comments")} className="m-0 grid list-none gap-3 p-0">
            {ticket.comments.map((c) => (
              <li key={c.id} className="grid gap-1 rounded-lg border bg-muted/30 p-3">
                <p className="m-0 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span className="font-medium text-foreground">{nameOf(c.author_id)}</span>
                  <time dateTime={c.created_at}>{formatDateTime(c.created_at, locale)}</time>
                  {c.internal && <Badge variant="outline">{t("helpdesk.comment.internalBadge")}</Badge>}
                </p>
                <p className="m-0 whitespace-pre-wrap text-sm">{c.body}</p>
              </li>
            ))}
          </ul>
        )}
        <form onSubmit={submit} noValidate className="grid gap-2">
          <label htmlFor="comment-body" className="text-sm font-medium">
            {t("helpdesk.comment.label")}
          </label>
          <Textarea
            id="comment-body"
            name="comment-body"
            rows={3}
            value={body}
            aria-invalid={missing || undefined}
            onChange={(e) => setBody(e.target.value)}
          />
          {missing && <p className="m-0 text-xs text-destructive">{t("helpdesk.comment.required")}</p>}
          {canWriteInternal(viewer) && (
            <label className="flex items-center gap-2 text-sm">
              <Checkbox checked={internal} onCheckedChange={(value) => setInternal(value === true)} />
              {t("helpdesk.comment.internal")}
            </label>
          )}
          <FormError message={comment.error ? t(errorKey(comment.error, { 403: "helpdesk.error.notAllowed" })) : null} />
          <div>
            <Button type="submit" disabled={comment.isPending}>
              <Send />
              {t("helpdesk.comment.send")}
            </Button>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}

function Field({ label, value, multiline }: { label: string; value: string; multiline?: boolean }) {
  return (
    <div className="grid gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={`m-0 text-sm ${multiline ? "whitespace-pre-wrap" : ""}`}>{value}</dd>
    </div>
  );
}
