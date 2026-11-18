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
import { Skeleton } from "@/components/ui/skeleton";
import { channelTitle, useNameOf } from "../chat/names";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { Page } from "../components/Page";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { useChat, useChatState } from "../state/chat";
import { useSessionState } from "../state/session";
import { useTaskApi } from "./api";
import { taskStatusTones } from "./format";
import { useMeta } from "../modules";

const statuses: TaskStatus[] = ["todo", "doing", "done"];
const dues: TaskDue[] = ["overdue", "today", "week"];
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
  const [status, setStatus] = useState<TaskStatus[]>([]);
  const [due, setDue] = useState<TaskDue | "">("");
  const [channelId, setChannelId] = useState("");

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

  const filtersActive = status.length > 0 || due !== "" || channelId !== "";

  function toggleStatus(s: TaskStatus) {
    setStatus((current) => statuses.filter((x) => (x === s ? !current.includes(s) : current.includes(x))));
  }

  function clearFilters() {
    setStatus([]);
    setDue("");
    setChannelId("");
  }

  const items = tasks.data?.pages.flatMap((page) => page.items) ?? [];
  const joined = Object.values(channels).map((s) => s.channel);

  return (
    <Page title={t("task.page.title")} width="wide">
      <div
        role="group"
        aria-label={t("task.page.filters")}
        className="flex flex-wrap items-end gap-x-6 gap-y-3 rounded-lg border bg-card p-3"
      >
        <fieldset className="m-0 grid min-w-0 gap-2 border-0 p-0">
          <legend className="mb-1 text-sm font-medium">{t("task.status")}</legend>
          <div className="flex flex-wrap gap-x-4 gap-y-2">
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
          </div>
        </fieldset>
        <Field label={t("task.filter.due")}>
          {({ id, describedBy }) => (
            <NativeSelect
              id={id}
              aria-describedby={describedBy}
              containerClassName="w-44"
              value={due}
              onChange={(e) => setDue(e.target.value as TaskDue | "")}
            >
              <option value="">{t("task.filter.any")}</option>
              {dues.map((d) => (
                <option key={d} value={d}>
                  {t(`task.due.${d}`)}
                </option>
              ))}
            </NativeSelect>
          )}
        </Field>
        <Field label={t("task.filter.channel")}>
          {({ id, describedBy }) => (
            <NativeSelect
              id={id}
              aria-describedby={describedBy}
              containerClassName="w-56"
              value={channelId}
              onChange={(e) => setChannelId(e.target.value)}
            >
              <option value="">{t("task.filter.allChannels")}</option>
              {joined.map((c) => (
                <option key={c.id} value={c.id}>
                  {channelTitle(c, chat?.userId ?? "", nameOf, t)}
                </option>
              ))}
            </NativeSelect>
          )}
        </Field>
        {filtersActive && (
          <Button type="button" variant="ghost" size="sm" onClick={clearFilters}>
            {t("task.page.clear")}
          </Button>
        )}
      </div>

      {tasks.isPending && <LoadingRows />}
      {tasks.isError && (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-destructive">
          <p className="m-0">{t("task.page.loadError")}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void tasks.refetch()}>
            {t("retry")}
          </Button>
        </div>
      )}
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
      {items.length > 0 && (
        <ul className="divide-y rounded-lg border bg-card">
          {items.map((task) => (
            <TaskRow key={task.id} task={task} channel={channels[task.channel_id]?.channel} />
          ))}
        </ul>
      )}
      {tasks.hasNextPage && (
        <div className="flex justify-center">
          <Button type="button" variant="outline" onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
            {t("loadMore")}
          </Button>
        </div>
      )}
    </Page>
  );
}

/** Placeholder rows shaped like the list, with the words for a screen reader. */
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

function TaskRow({ task, channel }: { task: Task; channel: Channel | undefined }) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  const tone = taskStatusTones[task.status];
  const due = task.due_at ? t("task.dueOn", { date: formatDateTime(task.due_at, locale) }) : t("task.noDue");
  return (
    <li className="grid gap-1 px-4 py-3 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center sm:gap-4">
      <div className="grid min-w-0 gap-0.5">
        <p className="m-0 font-medium break-words">{task.title}</p>
        <p className="m-0 text-xs text-muted-foreground">
          {due}
          {channel && (
            <>
              {" · "}
              <Link to={`/chat/${task.channel_id}`} className="text-foreground underline-offset-4 hover:underline">
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
    </li>
  );
}
