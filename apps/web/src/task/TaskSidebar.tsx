import { useQuery } from "@tanstack/react-query";
import { CircleCheck, Clock, Flame, Hash, ListChecks } from "lucide-react";
import { useMemo } from "react";
import { useLocation } from "react-router";
import { channelTitle, useNameOf } from "../chat/names";
import { SidebarLink, CountBadge } from "../components/SidebarLink";
import { SidebarSection } from "../components/SidebarSection";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { useChat, useChatState } from "../state/chat";
import { useSessionState } from "../state/session";
import { useTaskApi } from "./api";
import { openStatuses, parseTaskView, sameTaskView, taskViewPath, type TaskView } from "./view";

const open: Partial<TaskView> = { status: openStatuses };

/**
 * The sidebar of the Tasks app: the views of "My tasks" as links that set the page's filters in the
 * URL, then the channels my open tasks belong to. The channel list is one read of my open tasks
 * (the page's own endpoint), made while this sidebar shows.
 */
export function TaskSidebar() {
  const t = useT();
  const api = useTaskApi();
  const chat = useChat();
  const nameOf = useNameOf();
  const { channels } = useChatState();
  const { workspaceId } = useSessionState();
  const location = useLocation();
  const enabled = useModuleEnabled("task");
  const current = parseTaskView(new URLSearchParams(location.search));
  const onPage = location.pathname === "/tasks";

  const mine = useQuery({
    queryKey: ["my-tasks", workspaceId, "sidebar-channels"],
    queryFn: () => api.listTasks({ assignee: "me", status: openStatuses, limit: 100 }),
    enabled: enabled && workspaceId !== null,
  });

  const byChannel = useMemo(() => {
    const counts = new Map<string, number>();
    for (const task of mine.data?.items ?? []) counts.set(task.channel_id, (counts.get(task.channel_id) ?? 0) + 1);
    return [...counts.entries()]
      .map(([id, n]) => {
        const channel = channels[id]?.channel;
        return { id, n, title: channel ? channelTitle(channel, chat?.userId ?? "", nameOf, t) : "" };
      })
      .filter((c) => c.title !== "")
      .sort((a, b) => a.title.localeCompare(b.title));
  }, [mine.data, channels, chat?.userId, nameOf, t]);

  const views: { key: string; view: Partial<TaskView>; icon: typeof ListChecks; label: string }[] = [
    { key: "open", view: open, icon: ListChecks, label: t("nav.tasks") },
    { key: "today", view: { ...open, due: "today" }, icon: Clock, label: t("task.due.today") },
    { key: "overdue", view: { ...open, due: "overdue" }, icon: Flame, label: t("task.due.overdue") },
    { key: "done", view: { status: ["done"] }, icon: CircleCheck, label: t("task.status.done") },
  ];

  return (
    <div className="grid gap-2">
      <nav aria-label={t("task.views")} className="grid gap-0.5">
        {views.map((v) => (
          <SidebarLink key={v.key} to={taskViewPath(v.view)} icon={v.icon} current={onPage && sameTaskView(current, v.view)}>
            {v.label}
          </SidebarLink>
        ))}
      </nav>
      {byChannel.length > 0 && (
        <SidebarSection id="task-channels" title={t("task.byChannel")}>
          <nav aria-label={t("task.byChannel")} className="grid gap-0.5">
            {byChannel.map((c) => (
              <SidebarLink
                key={c.id}
                to={taskViewPath({ ...open, channelId: c.id })}
                icon={Hash}
                current={onPage && sameTaskView(current, { ...open, channelId: c.id })}
                trailing={<CountBadge n={c.n} label={t("task.openCount", { n: c.n })} />}
              >
                {c.title}
              </SidebarLink>
            ))}
          </nav>
        </SidebarSection>
      )}
    </div>
  );
}
