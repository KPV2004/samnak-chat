import type { HelpdeskQueue, Ticket, TicketPriority, TicketStatus } from "@samnak/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { LifeBuoy } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { priorityKeys, statuses, statusKeys, statusTones } from "./format";

const pageSize = 20;

interface Props {
  /** "mine" is the tickets I opened; "queue" is the tickets my queues handle. */
  role: "mine" | "queue";
  queues: HelpdeskQueue[];
}

/**
 * One list of tickets (HD-10). "My requests" asks the server for the tickets I
 * opened; the queue list is filtered by queue, status and assignee. The server
 * hides what the viewer may not see, so the page shows what it returns.
 */
export function TicketList({ role, queues }: Props) {
  const t = useT();
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
  const [queueId, setQueueId] = useState("");
  const [status, setStatus] = useState<TicketStatus | "">("");
  const [assignee, setAssignee] = useState("");

  const tickets = useInfiniteQuery({
    queryKey: ["helpdesk", "tickets", workspaceId, role, queueId, status, assignee],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      role === "mine"
        ? api.listTickets({ requester: "me", cursor: pageParam || undefined, limit: pageSize })
        : api.listTickets({
            queue_id: queueId || undefined,
            status: status ? [status] : undefined,
            assignee: assignee || undefined,
            cursor: pageParam || undefined,
            limit: pageSize,
          }),
    getNextPageParam: (last) => last.next_cursor || undefined,
  });

  const filtersActive = queueId !== "" || status !== "" || assignee !== "";
  const items = tickets.data?.pages.flatMap((page) => page.items) ?? [];
  const queueName = (id: string) => queues.find((q) => q.id === id)?.name ?? "";

  function clearFilters() {
    setQueueId("");
    setStatus("");
    setAssignee("");
  }

  return (
    <div className="grid gap-4">
      {role === "queue" && (
        <div
          role="group"
          aria-label={t("helpdesk.list.label")}
          className="flex flex-wrap items-end gap-x-4 gap-y-3 rounded-lg border bg-card p-3"
        >
          <Field label={t("helpdesk.queue")}>
            {({ id, describedBy }) => (
              <NativeSelect
                id={id}
                aria-describedby={describedBy}
                containerClassName="w-48"
                value={queueId}
                onChange={(e) => setQueueId(e.target.value)}
              >
                <option value="">{t("helpdesk.filter.anyQueue")}</option>
                {queues.map((q) => (
                  <option key={q.id} value={q.id}>
                    {q.name}
                  </option>
                ))}
              </NativeSelect>
            )}
          </Field>
          <Field label={t("helpdesk.status")}>
            {({ id, describedBy }) => (
              <NativeSelect
                id={id}
                aria-describedby={describedBy}
                containerClassName="w-44"
                value={status}
                onChange={(e) => setStatus(e.target.value as TicketStatus | "")}
              >
                <option value="">{t("helpdesk.filter.anyStatus")}</option>
                {statuses.map((s) => (
                  <option key={s} value={s}>
                    {t(statusKeys[s])}
                  </option>
                ))}
              </NativeSelect>
            )}
          </Field>
          <Field label={t("helpdesk.assignee")}>
            {({ id, describedBy }) => (
              <NativeSelect
                id={id}
                aria-describedby={describedBy}
                containerClassName="w-48"
                value={assignee}
                onChange={(e) => setAssignee(e.target.value)}
              >
                <option value="">{t("helpdesk.filter.anyone")}</option>
                <option value="me">{t("helpdesk.filter.me")}</option>
                <option value="none">{t("helpdesk.filter.none")}</option>
              </NativeSelect>
            )}
          </Field>
          {filtersActive && (
            <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
              {t("helpdesk.list.clear")}
            </Button>
          )}
        </div>
      )}

      {tickets.isPending && <LoadingRows />}
      {tickets.isError && (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-destructive">
          <p className="m-0">{t("helpdesk.list.loadError")}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void tickets.refetch()}>
            {t("retry")}
          </Button>
        </div>
      )}
      {tickets.isSuccess && items.length === 0 && (
        <EmptyState
          icon={LifeBuoy}
          title={t(role === "mine" ? "helpdesk.list.empty.mine" : "helpdesk.list.empty.queue")}
          action={
            role === "queue" && filtersActive ? (
              <Button type="button" variant="outline" size="sm" onClick={clearFilters}>
                {t("helpdesk.list.clear")}
              </Button>
            ) : undefined
          }
        />
      )}
      {items.length > 0 && (
        <ul aria-label={t("helpdesk.list.label")} className="m-0 list-none divide-y rounded-lg border bg-card p-0">
          {items.map((ticket) => (
            <TicketRow key={ticket.id} ticket={ticket} queue={queueName(ticket.queue_id)} />
          ))}
        </ul>
      )}
      {tickets.hasNextPage && (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="outline"
            onClick={() => void tickets.fetchNextPage()}
            disabled={tickets.isFetchingNextPage}
          >
            {t("loadMore")}
          </Button>
        </div>
      )}
    </div>
  );
}

function LoadingRows() {
  const t = useT();
  return (
    <div role="status" className="grid gap-2 rounded-lg border bg-card p-4">
      <span className="sr-only">{t("loading")}</span>
      {[0, 1, 2].map((i) => (
        <Skeleton key={i} className="h-10 w-full" />
      ))}
    </div>
  );
}

function TicketRow({ ticket, queue }: { ticket: Ticket; queue: string }) {
  const t = useT();
  const locale = useCurrentLocale();
  const tone = statusTones[ticket.status];
  return (
    <li className="grid gap-1 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-4">
      <div className="grid min-w-0 gap-0.5">
        <p className="m-0 font-medium break-words">
          <Link to={`/helpdesk/${ticket.id}`} className="underline-offset-4 hover:underline">
            {ticket.title}
          </Link>
        </p>
        <p className="m-0 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
          <span className="font-mono">{ticket.key}</span>
          {queue && <span>· {queue}</span>}
          <span>· {t("helpdesk.list.openedOn", { time: formatDateTime(ticket.created_at, locale) })}</span>
        </p>
      </div>
      <p className="m-0 flex flex-wrap items-center gap-1.5 text-sm">
        <Badge variant={tone.badge} className={tone.text}>
          {t(statusKeys[ticket.status])}
        </Badge>
        <Badge variant="outline">{t(priorityKeys[ticket.priority as TicketPriority])}</Badge>
      </p>
    </li>
  );
}
