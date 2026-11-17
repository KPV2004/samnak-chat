import type { Channel, Task, TaskDue, TaskStatus } from "@samnak/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Link, Navigate } from "react-router";
import { channelTitle, useNameOf } from "../chat/names";
import { Field } from "../components/Field";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { useChat, useChatState } from "../state/chat";
import { useSessionState } from "../state/session";
import { useTaskApi } from "./api";
import { useMeta } from "./modules";

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

  function toggleStatus(s: TaskStatus) {
    setStatus((current) => statuses.filter((x) => (x === s ? !current.includes(s) : current.includes(x))));
  }

  const items = tasks.data?.pages.flatMap((page) => page.items) ?? [];
  const joined = Object.values(channels).map((s) => s.channel);

  return (
    <section className="my-tasks">
      <h1>{t("task.page.title")}</h1>
      <div className="task-filters">
        <fieldset>
          <legend>{t("task.status")}</legend>
          {statuses.map((s) => (
            <label key={s} className="inline">
              <input type="checkbox" checked={status.includes(s)} onChange={() => toggleStatus(s)} /> {t(`task.status.${s}`)}
            </label>
          ))}
        </fieldset>
        <Field label={t("task.filter.due")}>
          {({ id, describedBy }) => (
            <select id={id} aria-describedby={describedBy} value={due} onChange={(e) => setDue(e.target.value as TaskDue | "")}>
              <option value="">{t("task.filter.any")}</option>
              {dues.map((d) => (
                <option key={d} value={d}>
                  {t(`task.due.${d}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <Field label={t("task.filter.channel")}>
          {({ id, describedBy }) => (
            <select id={id} aria-describedby={describedBy} value={channelId} onChange={(e) => setChannelId(e.target.value)}>
              <option value="">{t("task.filter.allChannels")}</option>
              {joined.map((c) => (
                <option key={c.id} value={c.id}>
                  {channelTitle(c, chat?.userId ?? "", nameOf, t)}
                </option>
              ))}
            </select>
          )}
        </Field>
      </div>

      {tasks.isPending && <p role="status">{t("loading")}</p>}
      {tasks.isError && (
        <p role="alert">
          {t("task.page.loadError")}{" "}
          <button type="button" onClick={() => void tasks.refetch()}>
            {t("retry")}
          </button>
        </p>
      )}
      {tasks.isSuccess && items.length === 0 && <p className="muted">{t("task.page.empty")}</p>}
      {items.length > 0 && (
        <ul className="task-list">
          {items.map((task) => (
            <TaskRow key={task.id} task={task} channel={channels[task.channel_id]?.channel} />
          ))}
        </ul>
      )}
      {tasks.hasNextPage && (
        <button type="button" onClick={() => void tasks.fetchNextPage()} disabled={tasks.isFetchingNextPage}>
          {t("loadMore")}
        </button>
      )}
    </section>
  );
}

function TaskRow({ task, channel }: { task: Task; channel: Channel | undefined }) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  return (
    <li className="task-row">
      <strong>{task.title}</strong>{" "}
      <span className="muted">
        {t(`task.status.${task.status}`)} · {t(`task.priority.${task.priority}`)}
      </span>
      <div>
        <small className="muted">
          {task.due_at ? t("task.dueOn", { date: formatDateTime(task.due_at, locale) }) : t("task.noDue")}
        </small>
        {channel && (
          <>
            {" "}
            <Link to={`/chat/${task.channel_id}`}>{channelTitle(channel, chat?.userId ?? "", nameOf, t)}</Link>
          </>
        )}
      </div>
    </li>
  );
}
