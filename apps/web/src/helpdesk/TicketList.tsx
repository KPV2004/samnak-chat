import type { HelpdeskQueue, Ticket, TicketPriority, TicketStatus } from "@samnak/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { LifeBuoy } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { DataList, DataRow, ListLoading, LoadMore, Toolbar, ToolbarField, stretchedLink } from "../pages/module-ui";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { priorityKeys, statuses, statusKeys, statusTones } from "./format";

const pageSize = 20;

interface Props {
  /** "mine" is the tickets I opened; "queue" is the tickets my queues handle. */
  role: "mine" | "queue";
  queues: HelpdeskQueue[];
  /** The queue the list is narrowed to, and how the toolbar changes it; both live in the URL. */
  queueId?: string;
  onQueueChange?: (id: string) => void;
}

/**
 * One list of tickets (HD-10). "My requests" asks the server for the tickets I
 * opened; the queue list is filtered by queue, status and assignee. The server
 * hides what the viewer may not see, so the page shows what it returns.
 */
export function TicketList({ role, queues, queueId = "", onQueueChange }: Props) {
  const t = useT();
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
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
    onQueueChange?.("");
    setStatus("");
    setAssignee("");
  }

  return (
    <div className="grid gap-4">
      {role === "queue" && (
        <Toolbar label={t("helpdesk.list.label")}>
          <ToolbarField label={t("helpdesk.queue")}>
            {(id) => (
              <NativeSelect id={id} size="sm" containerClassName="max-w-48" value={queueId} onChange={(e) => onQueueChange?.(e.target.value)}>
                <option value="">{t("helpdesk.filter.anyQueue")}</option>
                {queues.map((q) => (
                  <option key={q.id} value={q.id}>
                    {q.name}
                  </option>
                ))}
              </NativeSelect>
            )}
          </ToolbarField>
          <ToolbarField label={t("helpdesk.status")}>
            {(id) => (
              <NativeSelect id={id} size="sm" value={status} onChange={(e) => setStatus(e.target.value as TicketStatus | "")}>
                <option value="">{t("helpdesk.filter.anyStatus")}</option>
                {statuses.map((s) => (
                  <option key={s} value={s}>
                    {t(statusKeys[s])}
                  </option>
                ))}
              </NativeSelect>
            )}
          </ToolbarField>
          <ToolbarField label={t("helpdesk.assignee")}>
            {(id) => (
              <NativeSelect id={id} size="sm" value={assignee} onChange={(e) => setAssignee(e.target.value)}>
                <option value="">{t("helpdesk.filter.anyone")}</option>
                <option value="me">{t("helpdesk.filter.me")}</option>
                <option value="none">{t("helpdesk.filter.none")}</option>
              </NativeSelect>
            )}
          </ToolbarField>
          {filtersActive && (
            <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
              {t("helpdesk.list.clear")}
            </Button>
          )}
        </Toolbar>
      )}

      {tickets.isPending && <ListLoading />}
      {tickets.isError && <AccountLoadError message={t("helpdesk.list.loadError")} onRetry={() => void tickets.refetch()} />}
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
        <DataList aria-label={t("helpdesk.list.label")}>
          {items.map((ticket) => (
            <TicketRow key={ticket.id} ticket={ticket} queue={queueName(ticket.queue_id)} />
          ))}
        </DataList>
      )}
      {tickets.hasNextPage && <LoadMore loading={tickets.isFetchingNextPage} onClick={() => void tickets.fetchNextPage()} />}
    </div>
  );
}

function TicketRow({ ticket, queue }: { ticket: Ticket; queue: string }) {
  const t = useT();
  const locale = useCurrentLocale();
  const tone = statusTones[ticket.status];
  return (
    <DataRow interactive>
      <div className="grid min-w-0 gap-0.5">
        <p className="m-0 font-medium break-words">
          <Link to={`/helpdesk/${ticket.id}`} className={stretchedLink}>
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
    </DataRow>
  );
}
