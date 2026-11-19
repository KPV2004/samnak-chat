import { parseTaskCard, type Message, type Task, type TaskCardPayload, type TaskStatus } from "@samnak/core";
import { useState } from "react";
import { useSearchParams } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { useNameOf } from "../chat/names";
import { useCurrentLocale, useT } from "../i18n/useT";
import type { MessageKey } from "../i18n/messages";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { CardFrame, FallbackCard } from "../pages/module-ui";
import { useChat } from "../state/chat";
import { useTaskApi } from "./api";
import { useModuleEnabled } from "../modules";
import { taskStatusTones } from "./format";

const statuses: TaskStatus[] = ["todo", "doing", "done"];

/**
 * The `task.item` card (TASK-03): a compact attachment with the title, who has the task,
 * the priority, the due date and the status, which can be changed here. "Comments" opens
 * the card's thread, which holds the task's comments (TASK-07). Without the task module
 * the card shows its fallback text.
 */
export function TaskCard({ message }: { message: Message }) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const api = useTaskApi();
  const nameOf = useNameOf();
  const enabled = useModuleEnabled("task");
  const [, setParams] = useSearchParams();
  const [saving, setSaving] = useState(false);
  const [failed, setFailed] = useState<MessageKey | null>(null);
  const card = parseTaskCard(message.payload);

  if (!enabled || !card) {
    return <FallbackCard message={message} />;
  }

  const taskId = card.task_id;

  /**
   * Puts the task's current state into the card, unless the card already holds a
   * newer version (a later `card.updated` that arrived first).
   */
  function applyTask(task: Task) {
    const latest = chat?.store.getState().channels[message.channel_id]?.messages.find((m) => m.id === message.id);
    const shown = parseTaskCard(latest?.payload ?? message.payload);
    if (!chat || !latest || !shown || shown.version > Date.parse(task.updated_at)) return;
    chat.store.applyMessage({ ...latest, payload: withTask(shown, task) });
  }

  async function changeStatus(status: TaskStatus) {
    setSaving(true);
    setFailed(null);
    try {
      applyTask(await api.updateTask(taskId, { status }));
    } catch (err) {
      setFailed(errorKey(err));
      // The server refused: show what it has, not what the card had.
      await api.getTask(taskId).then(applyTask, () => undefined);
    } finally {
      setSaving(false);
    }
  }

  function openComments() {
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set("thread", message.id);
      return next;
    });
  }

  const assignee = card.assignee_name || (card.assignee_id ? nameOf(card.assignee_id) : t("task.unassigned"));
  const due = card.due_at ? t("task.dueOn", { date: formatDateTime(card.due_at, locale) }) : t("task.noDue");
  const tone = taskStatusTones[card.status];

  return (
    <CardFrame
      kind={t("task.label")}
      bar={tone.bar}
      badge={
        <Badge variant={tone.badge} className={tone.text}>
          {t(`task.status.${card.status}`)}
        </Badge>
      }
      title={card.title}
      meta={
        <ul className="m-0 flex list-none flex-wrap items-center gap-x-3 gap-y-1 p-0">
          <li>{assignee}</li>
          <li>
            <Badge variant="outline">{t(`task.priority.${card.priority}`)}</Badge>
          </li>
          <li>{due}</li>
        </ul>
      }
      actions={
        <>
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <span>{t("task.status")}</span>
            <NativeSelect
              size="sm"
              value={card.status}
              disabled={saving}
              onChange={(e) => void changeStatus(e.target.value as TaskStatus)}
            >
              {statuses.map((s) => (
                <option key={s} value={s}>
                  {t(`task.status.${s}`)}
                </option>
              ))}
            </NativeSelect>
          </label>
          <Button type="button" variant="ghost" size="sm" className="ml-auto" onClick={openComments}>
            {t("task.comments")}
          </Button>
        </>
      }
    >
      {failed && (
        <p role="alert" className="m-0 text-xs text-destructive">
          {t(failed)}
        </p>
      )}
    </CardFrame>
  );
}

/** The card's payload with the task's current fields. The assignee's name is kept only while the assignee is the same. */
function withTask(card: TaskCardPayload, task: Task): TaskCardPayload {
  return {
    ...card,
    title: task.title,
    status: task.status,
    priority: task.priority,
    assignee_id: task.assignee_id,
    assignee_name: task.assignee_id === card.assignee_id ? card.assignee_name : "",
    due_at: task.due_at,
    version: Date.parse(task.updated_at),
  };
}
