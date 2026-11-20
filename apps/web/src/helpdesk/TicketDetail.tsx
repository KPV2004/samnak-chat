import type { HelpdeskQueue, TicketDetail as TicketDetailData, TicketStatus } from "@samnak/core";
import { canAssign, canClaim, canEditTicket, canWriteInternal, statusMoves, type Viewer } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Paperclip, Send } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useParams } from "react-router";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { NativeSelect } from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { BackLink as BackToList, DetailLayout, DetailLoading, Panel, Properties, Property } from "../pages/module-ui";
import { useModuleEnabled } from "../modules";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSessionState } from "../state/session";
import { useHelpdeskApi, useHelpdeskFileApi, useTaskApi } from "./api";
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
      <Page title={t("helpdesk.title")} width="wide" actions={<BackLink />}>
        <DetailLoading />
      </Page>
    );
  }
  if (ticket.isError) {
    return (
      <Page title={t("helpdesk.title")} width="wide" actions={<BackLink />}>
        <FormError message={t(errorKey(ticket.error, { 404: "helpdesk.notFound" }))} />
      </Page>
    );
  }
  return <Detail ticket={ticket.data} queues={queues.data?.items ?? []} />;
}

function BackLink() {
  const t = useT();
  return <BackToList to="/helpdesk">{t("helpdesk.back")}</BackToList>;
}

function Detail({ ticket, queues }: { ticket: TicketDetailData; queues: HelpdeskQueue[] }) {
  const t = useT();
  const locale = useCurrentLocale();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const me = useMe().data?.id ?? "";
  const workspace = useWorkspaces().current;
  const nameOf = useNameOf();
  const taskEnabled = useModuleEnabled("task");
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
    mutationFn: (patch: { priority?: (typeof priorities)[number]; category_id?: string }) =>
      api.editTicket(ticket.id, patch),
    onSuccess: refresh,
  });
  const moves = statusMoves(ticket, viewer, new Date());
  const actionError = claim.error ?? assign.error ?? move.error ?? edit.error;

  const canAct = canClaim(ticket, viewer) || canAssign(ticket, viewer) || moves.length > 0 || canEditTicket(viewer);

  return (
    <Page
      width="wide"
      title={
        <span className="inline-flex flex-wrap items-baseline gap-2">
          <span className="font-mono text-base text-muted-foreground">{ticket.key}</span>
          <span>{ticket.title}</span>
        </span>
      }
      actions={
        <>
          {taskEnabled && queue?.channel_id && <CreateTaskButton ticket={ticket} channelId={queue.channel_id} />}
          <BackLink />
        </>
      }
    >
      <DetailLayout
        asideLabel={t("helpdesk.properties")}
        aside={
          <>
            <Panel title={t("helpdesk.properties")}>
              <Properties>
                <Property label={t("helpdesk.status")}>
                  <Badge variant={tone.badge} className={tone.text}>
                    {t(statusKeys[ticket.status])}
                  </Badge>
                </Property>
                <Property label={t("helpdesk.priority")}>{t(priorityKeys[ticket.priority])}</Property>
                {queue && <Property label={t("helpdesk.queue")}>{queue.name}</Property>}
                <Property label={t("helpdesk.requester")}>{nameOf(ticket.requester_id)}</Property>
                <Property label={t("helpdesk.assignee")}>
                  {ticket.assignee_id ? nameOf(ticket.assignee_id) : t("helpdesk.unassigned")}
                </Property>
                <Property label={t("helpdesk.category")}>{categoryName(ticket.category_id)}</Property>
                <Property label={t("helpdesk.opened")}>{formatDateTime(ticket.created_at, locale)}</Property>
                <Property label={t("helpdesk.updated")}>{formatDateTime(ticket.updated_at, locale)}</Property>
              </Properties>
            </Panel>
            {canAct && (
              <Panel title={t("helpdesk.detail.moveTo")}>
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
                  <label className="grid gap-1.5 text-sm">
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
                  <>
                    <label className="grid gap-1.5 text-sm">
                      <span className="font-medium">{t("helpdesk.priority")}</span>
                      <NativeSelect
                        containerClassName="w-full"
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
                    <label className="grid gap-1.5 text-sm">
                      <span className="font-medium">{t("helpdesk.category")}</span>
                      <NativeSelect
                        containerClassName="w-full"
                        value={ticket.category_id}
                        onChange={(e) =>
                          e.target.value !== ticket.category_id && edit.mutate({ category_id: e.target.value })
                        }
                      >
                        {categories.map((c) => (
                          <option key={c.id} value={c.id}>
                            {c.name}
                          </option>
                        ))}
                      </NativeSelect>
                    </label>
                  </>
                )}
                <FormError
                  message={
                    actionError
                      ? t(
                          errorKey(actionError, {
                            409: "helpdesk.error.invalidTransition",
                            403: "helpdesk.error.notAllowed",
                          }),
                        )
                      : null
                  }
                />
              </Panel>
            )}
          </>
        }
      >
        <Panel title={t("helpdesk.details")}>
          {ticket.description ? (
            <p className="m-0 whitespace-pre-wrap text-sm">{ticket.description}</p>
          ) : (
            <p className="m-0 text-sm text-muted-foreground">—</p>
          )}
          {ticket.attachments.length > 0 && (
            <section className="grid gap-1.5">
              <h3 className="m-0 text-sm font-medium">{t("helpdesk.detail.attachments")}</h3>
              <ul className="m-0 grid list-none gap-1 p-0">
                {ticket.attachments.map((a) => (
                  <li key={a.id} className="flex flex-wrap items-center gap-2 text-sm">
                    <Paperclip className="size-4 text-muted-foreground" aria-hidden="true" />
                    <span className="min-w-0 break-all">{a.file_name}</span>
                    <FileDownload id={a.id} name={a.file_name} />
                  </li>
                ))}
              </ul>
            </section>
          )}
        </Panel>

        <CommentSection ticket={ticket} viewer={viewer} />

        <Panel title={t("helpdesk.history")}>
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
        </Panel>
      </DetailLayout>
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
    <Panel title={t("helpdesk.comments")}>
      {ticket.comments.length === 0 ? (
        <p className="m-0 text-sm text-muted-foreground">{t("helpdesk.comment.empty")}</p>
      ) : (
        <ul aria-label={t("helpdesk.comments")} className="m-0 grid list-none gap-3 p-0">
          {ticket.comments.map((c) => (
            <li key={c.id} className="grid gap-1 rounded-md bg-muted/50 p-3">
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
    </Panel>
  );
}

/** A file of the ticket: a button that opens its download link in a new tab (HD-03). */
function FileDownload({ id, name }: { id: string; name: string }) {
  const t = useT();
  const files = useHelpdeskFileApi();
  const download = useMutation({
    mutationFn: () => files.attachmentUrl(id),
    onSuccess: ({ url }) => {
      window.open(url, "_blank", "noopener");
    },
  });
  return (
    <>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        aria-label={t("helpdesk.detail.downloadFile", { name })}
        disabled={download.isPending}
        onClick={() => download.mutate()}
      >
        {t("helpdesk.detail.download")}
      </Button>
      <FormError message={download.error ? t(errorKey(download.error)) : null} />
    </>
  );
}

/**
 * Creates a task in the queue's channel that names the ticket (HD-19). The server posts the task's
 * card there; the button is shown only when `/meta` lists task and the queue has a channel.
 */
function CreateTaskButton({
  ticket,
  channelId,
}: {
  ticket: Pick<TicketDetailData, "key" | "title">;
  channelId: string;
}) {
  const t = useT();
  const tasks = useTaskApi();
  const create = useMutation({
    mutationFn: () =>
      tasks.createTask({
        channel_id: channelId,
        // A task title is at most 200 characters.
        title: [...`${ticket.key}: ${ticket.title}`].slice(0, 200).join(""),
        description: `Helpdesk ticket ${ticket.key}.`,
      }),
    onSuccess: () => {
      toast.success(t("helpdesk.detail.taskCreated"));
    },
  });
  return (
    <>
      <Button type="button" variant="outline" disabled={create.isPending} onClick={() => create.mutate()}>
        {t("helpdesk.detail.createTask")}
      </Button>
      <FormError message={create.error ? t(errorKey(create.error)) : null} />
    </>
  );
}
