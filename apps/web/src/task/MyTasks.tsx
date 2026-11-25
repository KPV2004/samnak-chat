import type { Channel, Task, TaskDue, TaskStatus } from "@samnak/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { ListChecks } from "lucide-react";
import { useId, useState } from "react";
import { Link, Navigate } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { NativeSelect } from "@/components/ui/native-select";
import { channelTitle, useNameOf } from "../chat/names";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { ListSearch, matchesQuery } from "../components/ListSearch";
import { Page } from "../components/Page";
import { useCurrentLocale, useT } from "../i18n/useT";
import { DataList, DataRow, ListLoading, LoadMore, Toolbar, ToolbarField } from "../pages/module-ui";
import { formatDateTime } from "../lib/format";
import { useChat, useChatState } from "../state/chat";
import { useSessionState } from "../state/session";
import { useTaskApi } from "./api";
import { taskStatusTones } from "./format";
import { useMeta } from "../modules";
import { openStatuses, taskDues, taskStatuses, useTaskView } from "./view";

const statuses = taskStatuses;
const dues = taskDues;
const pageSize = 20;

/**
 * "My tasks" (TASK-04): the tasks assigned to me, filtered by status, due date and
 * channel. Without the task module the page is not there: it leads back home.
 */
export function MyTasks() {
  const t = useT();
  const meta = useMeta();
  if (meta.isPending) return <p role="status">{t("loading")}</p>;
  if (!meta.data?.modules.includes("task")) return <Navigate to="/" replace />;
  return <TaskList />;
}

function TaskList() {
  const t = useT();
  const api = useTaskApi();
  const chat = useChat();
  const nameOf = useNameOf();
  const { channels } = useChatState();
  const { workspaceId } = useSessionState();
  const filterId = useId();
  // The filters live in the URL, so the sidebar's views, this toolbar, back and reload all agree.
  const [{ status, due, channelId }, setView] = useTaskView();

  const tasks = useInfiniteQuery({
    // A workspace has its own tasks, so its list is never shown for another.
    queryKey: ["my-tasks", workspaceId, status, due, channelId],
    initialPageParam: "",
    queryFn: ({ pageParam }) =>
      api.listTasks({
        assignee: "me",
        status: status.length > 0 ? status : undefined,
        due: due || undefined,
        channel_id: channelId || undefined,
        cursor: pageParam || undefined,
        limit: pageSize,
      }),
    getNextPageParam: (last) => last.next_cursor || undefined,
  });

  const filtersActive = status.join(",") !== openStatuses.join(",") || due !== "" || channelId !== "";

  function toggleStatus(s: TaskStatus) {
    setView({ status: statuses.filter((x) => (x === s ? !status.includes(s) : status.includes(x))) });
  }

  function clearFilters() {
    setView({ status: openStatuses, due: "", channelId: "" });
  }

  const items = tasks.data?.pages.flatMap((page) => page.items) ?? [];
  const [query, setQuery] = useState("");
  const shown = items.filter((task) => matchesQuery(query, task.title, task.description));
  const joined = Object.values(channels).map((s) => s.channel);

  return (
    <Page title={t("task.page.title")} width="wide">
      <Toolbar label={t("task.page.filters")}>
        <fieldset className="m-0 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2 border-0 p-0">
          <legend className="sr-only">{t("task.status")}</legend>
          <span aria-hidden="true" className="text-sm text-muted-foreground">
            {t("task.status")}
          </span>
          {statuses.map((s) => {
            const id = `${filterId}-${s}`;
            return (
              <div key={s} className="flex items-center gap-2">
                <Checkbox id={id} checked={status.includes(s)} onCheckedChange={() => toggleStatus(s)} />
                <Label htmlFor={id} className="leading-normal font-normal">
                  {t(`task.status.${s}`)}
                </Label>
              </div>
            );
          })}
        </fieldset>
        <ToolbarField label={t("task.filter.due")}>
          {(id) => (
            <NativeSelect id={id} size="sm" value={due} onChange={(e) => setView({ due: e.target.value as TaskDue | "" })}>
              <option value="">{t("task.filter.any")}</option>
              {dues.map((d) => (
                <option key={d} value={d}>
                  {t(`task.due.${d}`)}
                </option>
              ))}
            </NativeSelect>
          )}
        </ToolbarField>
        <ToolbarField label={t("task.filter.channel")}>
          {(id) => (
            <NativeSelect id={id} size="sm" containerClassName="max-w-56" value={channelId} onChange={(e) => setView({ channelId: e.target.value })}>
              <option value="">{t("task.filter.allChannels")}</option>
              {joined.map((c) => (
                <option key={c.id} value={c.id}>
                  {channelTitle(c, chat?.userId ?? "", nameOf, t)}
                </option>
              ))}
            </NativeSelect>
          )}
        </ToolbarField>
        {filtersActive && (
          <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
            {t("task.page.clear")}
          </Button>
        )}
        <ListSearch value={query} onChange={setQuery} count={items.length} />
      </Toolbar>

      {tasks.isPending && <ListLoading />}
      {tasks.isError && <AccountLoadError message={t("task.page.loadError")} onRetry={() => void tasks.refetch()} />}
      {tasks.isSuccess && items.length === 0 && (
        <EmptyState
          icon={ListChecks}
          title={t("task.page.empty")}
          action={
            filtersActive ? (
              <Button type="button" variant="outline" size="sm" onClick={clearFilters}>
                {t("task.page.clear")}
              </Button>
            ) : undefined
          }
        />
      )}
      {items.length > 0 && shown.length === 0 && (
        <p role="status" className="m-0 text-sm text-muted-foreground">
          {t("list.search.empty", { n: items.length })}{" "}
          <Button type="button" variant="link" size="sm" onClick={() => setQuery("")}>
            {t("list.search.clear")}
          </Button>
        </p>
      )}
      {shown.length > 0 && (
        <DataList>
          {shown.map((task) => (
            <TaskRow key={task.id} task={task} channel={channels[task.channel_id]?.channel} />
          ))}
        </DataList>
      )}
      {tasks.hasNextPage && <LoadMore loading={tasks.isFetchingNextPage} onClick={() => void tasks.fetchNextPage()} />}
    </Page>
  );
}

function TaskRow({ task, channel }: { task: Task; channel: Channel | undefined }) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  const tone = taskStatusTones[task.status];
  const due = task.due_at ? t("task.dueOn", { date: formatDateTime(task.due_at, locale) }) : t("task.noDue");
  return (
    <DataRow>
      <div className="grid min-w-0 gap-0.5">
        <p className="m-0 font-medium break-words">{task.title}</p>
        <p className="m-0 text-xs text-muted-foreground">
          {due}
          {channel && (
            <>
              {" · "}
              <Link to={`/chat/${task.channel_id}`} className="text-primary underline-offset-4 hover:underline">
                {channelTitle(channel, chat?.userId ?? "", nameOf, t)}
              </Link>
            </>
          )}
        </p>
      </div>
      <p className="m-0 flex flex-wrap items-center gap-1.5 text-sm text-muted-foreground">
        <Badge variant={tone.badge} className={tone.text}>
          {t(`task.status.${task.status}`)}
        </Badge>{" "}
        · {t(`task.priority.${task.priority}`)}
      </p>
    </DataRow>
  );
}
