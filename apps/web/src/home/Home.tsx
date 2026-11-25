import { createM3Api, type Task, type TaskStatus, type Ticket } from "@samnak/core";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { Check, ClipboardCheck, LifeBuoy, ListChecks, MessagesSquare, X } from "lucide-react";
import { Link, useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Page } from "@/components/Page";
import { useT } from "@/i18n/useT";
import { useModuleEnabled } from "@/modules";
import { summarize } from "../approval/format";
import { statusKeys as ticketStatusKeys } from "../helpdesk/format";
import { useApprovalRequests } from "../approval/requests";
import { useQuickApprove } from "../approval/useQuickApprove";
import { channelTitle, useNameOf } from "../chat/names";
import { useChat, useChatState } from "../state/chat";
import { useSession, useSessionState } from "../state/session";
import { useHelpdeskApi } from "../helpdesk/api";
import { useTaskApi } from "../task/api";
import { SummaryCard, SummaryRow } from "./SummaryCard";

/** Tasks due today or overdue, from the same list query (and key) as the Tasks page. */
const openStatuses: TaskStatus[] = ["todo", "doing"];
const taskStatusKeys: Record<TaskStatus, "task.status.todo" | "task.status.doing" | "task.status.done"> = {
  todo: "task.status.todo",
  doing: "task.status.doing",
  done: "task.status.done",
};

function useDueTasks(due: "today" | "overdue", enabled: boolean) {
  const api = useTaskApi();
  const { workspaceId } = useSessionState();
  return useInfiniteQuery({
    queryKey: ["my-tasks", workspaceId, openStatuses, due, ""],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api.listTasks({
        assignee: "me",
        status: openStatuses,
        due,
        cursor: pageParam || undefined,
        limit: 20,
      }),
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: enabled && workspaceId !== null,
  });
}

function useMyTickets(enabled: boolean) {
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
  return useInfiniteQuery({
    // The same key as the Helpdesk "My requests" list, so both show one answer.
    queryKey: ["helpdesk", "tickets", workspaceId, "mine", "", "", ""],
    initialPageParam: "",
    queryFn: ({ pageParam }) => api.listTickets({ requester: "me", cursor: pageParam || undefined, limit: 20 }),
    getNextPageParam: (last) => last.next_cursor || undefined,
    enabled: enabled && workspaceId !== null,
  });
}

const openTicket = (t: Ticket) => t.status !== "resolved" && t.status !== "closed";

/** Home: one place for what is waiting across chat, tasks, approvals and helpdesk. */
export function Home() {
  const t = useT();
  const approvalOn = useModuleEnabled("approval");
  const taskOn = useModuleEnabled("task");
  const helpdeskOn = useModuleEnabled("helpdesk");
  const notifyOn = useModuleEnabled("notify");

  const approvals = useApprovalRequests("approver", approvalOn);
  const today = useDueTasks("today", taskOn);
  const overdue = useDueTasks("overdue", taskOn);
  const tickets = useMyTickets(helpdeskOn);
  const quickApprove = useQuickApprove();
  const navigate = useNavigate();
  const { channels } = useChatState();
  const me = useChat()?.userId ?? "";
  const nameOf = useNameOf();

  const approvalItems = approvals.data?.pages.flatMap((p) => p.items) ?? [];
  const taskItems = [...(today.data?.pages.flatMap((p) => p.items) ?? []), ...(overdue.data?.pages.flatMap((p) => p.items) ?? [])] as Task[];
  const ticketItems = (tickets.data?.pages.flatMap((p) => p.items) ?? []).filter(openTicket);
  const unread = Object.values(channels)
    .map((s) => s.channel)
    .filter((c) => (channels[c.id]?.unreadCount ?? 0) > 0)
    .sort((a, b) => (channels[b.id]?.unreadCount ?? 0) - (channels[a.id]?.unreadCount ?? 0));
  const chatCount = unread.reduce((n, c) => n + (channels[c.id]?.unreadCount ?? 0), 0);

  const session = useSession();
  const activity = useQuery({
    queryKey: ["home", "activity"],
    enabled: notifyOn,
    queryFn: () => createM3Api(session.client).listNotifications({ limit: 10 }),
  });

  return (
    <Page title={t("nav.home")} description={t("home.waiting")} width="wide">
      <div className="grid gap-4 md:grid-cols-2">
        {approvalOn && (
          <SummaryCard
            title={t("home.card.approvals")}
            icon={ClipboardCheck}
            count={approvalItems.length}
            more={approvals.hasNextPage}
            href="/approvals"
            seeAll={t("home.seeAll")}
            empty={t("approval.noneToApprove")}
          >
            {approvalItems.slice(0, 3).map((r) => (
              <SummaryRow
                key={r.id}
                to={`/approvals/${r.id}`}
                title={summarize(r.type, r.payload, t)}
                subtitle={nameOf(r.requester_id)}
                trailing={
                  <span className="flex shrink-0 gap-1">
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => quickApprove(r)}
                      aria-label={`${t("approval.approve")} ${summarize(r.type, r.payload, t)}`}
                    >
                      <Check aria-hidden="true" />
                      {t("approval.approve")}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => navigate(`/approvals/${r.id}`)}
                      aria-label={`${t("approval.reject")} ${summarize(r.type, r.payload, t)}`}
                    >
                      <X aria-hidden="true" />
                    </Button>
                  </span>
                }
              />
            ))}
          </SummaryCard>
        )}

        {taskOn && (
          <SummaryCard
            title={t("home.card.tasks")}
            icon={ListChecks}
            count={taskItems.length}
            more={today.hasNextPage || overdue.hasNextPage}
            href="/tasks"
            seeAll={t("home.seeAll")}
            empty={t("home.empty")}
          >
            {taskItems.slice(0, 3).map((task) => (
              <SummaryRow key={task.id} to={`/tasks?task=${task.id}`} title={task.title} subtitle={t(taskStatusKeys[task.status])} />
            ))}
          </SummaryCard>
        )}

        {helpdeskOn && (
          <SummaryCard
            title={t("home.card.tickets")}
            icon={LifeBuoy}
            count={ticketItems.length}
            more={tickets.hasNextPage}
            href="/helpdesk"
            seeAll={t("home.seeAll")}
            empty={t("home.empty")}
          >
            {ticketItems.slice(0, 3).map((ticket) => (
              <SummaryRow
                key={ticket.id}
                to={`/helpdesk/${ticket.id}`}
                title={`${ticket.key} · ${ticket.title}`}
                subtitle={t(ticketStatusKeys[ticket.status])}
              />
            ))}
          </SummaryCard>
        )}

        <SummaryCard
          title={t("home.card.chats")}
          icon={MessagesSquare}
          count={chatCount}
          more={false}
          href="/chat"
          seeAll={t("home.seeAll")}
          empty={t("home.empty")}
        >
          {unread.slice(0, 3).map((c) => (
            <SummaryRow
              key={c.id}
              to={`/chat/${c.id}`}
              title={channelTitle(c, me, nameOf, t)}
              subtitle={t("chat.unread", { n: channels[c.id]?.unreadCount ?? 0 })}
            />
          ))}
        </SummaryCard>
      </div>

      {notifyOn && (
        <section className="grid gap-2">
          <h2 className="m-0 text-base font-semibold">{t("home.recent")}</h2>
          <ul className="divide-y rounded-lg border bg-card">
            {(activity.data?.items ?? []).slice(0, 10).map((n) => (
              <li key={n.id}>
                <Link to={n.link} className="flex items-start gap-3 p-3 text-sm hover:bg-accent/60">
                  <span className="min-w-0 flex-1">
                    <span className="block font-medium">{n.title}</span>
                    <span className="block truncate text-xs text-muted-foreground">{n.body}</span>
                  </span>
                  {!n.read_at && <span className="mt-1.5 size-2 shrink-0 rounded-full bg-primary" aria-hidden="true" />}
                  <span className="shrink-0 text-xs text-muted-foreground">{n.created_at.slice(0, 10)}</span>
                </Link>
              </li>
            ))}
            {activity.data && activity.data.items.length === 0 && <li className="p-3 text-sm text-muted-foreground">{t("home.empty")}</li>}
          </ul>
        </section>
      )}
    </Page>
  );
}
