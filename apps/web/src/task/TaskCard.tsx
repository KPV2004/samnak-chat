import { parseTaskCard, type Message, type Task, type TaskCardPayload, type TaskStatus } from "@samnak/core";
import { useState } from "react";
import { useSearchParams } from "react-router";
import { useNameOf } from "../chat/names";
import { useCurrentLocale, useT } from "../i18n/useT";
import type { MessageKey } from "../i18n/messages";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useChat } from "../state/chat";
import { useTaskApi } from "./api";
import { useModuleEnabled } from "./modules";
import "./task.css";

const statuses: TaskStatus[] = ["todo", "doing", "done"];

/**
 * The `task.item` card (TASK-03): the title, who has the task, the priority, the
 * due date and the status, which can be changed here. "Comments" opens the card's
 * thread, which holds the task's comments (TASK-07). Without the task module the
 * card shows its fallback text.
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
    return (
      <div className="chat-card">
        <small className="muted">{t("chat.card")}</small>
        <div>{message.fallback_text || message.body}</div>
      </div>
    );
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
    // The generated schema types a card payload as an empty object; the card's kind decides its shape.
    chat.store.applyMessage({ ...latest, payload: withTask(shown, task) as Message["payload"] });
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

  return (
    <div className="chat-card task-card">
      <small className="muted">{t("task.label")}</small>
      <div>
        <strong>{card.title}</strong>
      </div>
      <ul className="task-card-meta">
        <li>{assignee}</li>
        <li>{t(`task.priority.${card.priority}`)}</li>
        <li>{due}</li>
      </ul>
      <div className="task-card-actions">
        <label className="inline">
          {t("task.status")}{" "}
          <select value={card.status} disabled={saving} onChange={(e) => void changeStatus(e.target.value as TaskStatus)}>
            {statuses.map((s) => (
              <option key={s} value={s}>
                {t(`task.status.${s}`)}
              </option>
            ))}
          </select>
        </label>
        <button type="button" onClick={openComments}>
          {t("task.comments")}
        </button>
      </div>
      {failed && (
        <p className="form-error" role="alert">
          {t(failed)}
        </p>
      )}
    </div>
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
