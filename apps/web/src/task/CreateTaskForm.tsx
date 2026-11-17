import type { Message, TaskPriority } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useNameOf } from "../chat/names";
import { Field } from "../components/Field";
import { useT } from "../i18n/useT";
import type { MessageKey } from "../i18n/messages";
import { errorKey } from "../lib/errors";
import { useChat } from "../state/chat";
import { useTaskApi } from "./api";
import "./task.css";

const maxTitle = 200;
const priorities: TaskPriority[] = ["low", "normal", "high"];

interface Props {
  /** The message the task is made from: its text is the default title and its ID is the source. */
  message: Message;
  channelId: string;
  onDone(): void;
}

/**
 * The "Create task" form of a message (TASK-01). The server posts the `task.item`
 * card in the channel when the task is created, so the form only closes.
 */
export function CreateTaskForm({ message, channelId, onDone }: Props) {
  const t = useT();
  const chat = useChat();
  const api = useTaskApi();
  const nameOf = useNameOf();
  const [title, setTitle] = useState(() => [...message.body.trim()].slice(0, maxTitle).join(""));
  const [assigneeId, setAssigneeId] = useState("");
  const [due, setDue] = useState("");
  const [priority, setPriority] = useState<TaskPriority>("normal");
  const [titleError, setTitleError] = useState<MessageKey | null>(null);
  const [failed, setFailed] = useState<MessageKey | null>(null);
  const [saving, setSaving] = useState(false);

  const members = useQuery({
    queryKey: ["chat-channel-members", channelId],
    enabled: chat !== null,
    staleTime: 30_000,
    queryFn: async () => (await chat!.api.listChannelMembers(channelId)).items,
  });

  async function submit(e: FormEvent) {
    e.preventDefault();
    const clean = title.trim();
    if (clean === "" || [...clean].length > maxTitle) {
      setTitleError(clean === "" ? "task.titleRequired" : "task.titleHint");
      return;
    }
    setTitleError(null);
    setFailed(null);
    setSaving(true);
    try {
      await api.createTask({
        channel_id: channelId,
        title: clean,
        source_message_id: message.id,
        priority,
        ...(assigneeId ? { assignee_id: assigneeId } : {}),
        // The end of the chosen day, in the sender's time zone.
        ...(due ? { due_at: new Date(`${due}T23:59:59`).toISOString() } : {}),
      });
      onDone();
    } catch (err) {
      setFailed(errorKey(err));
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="task-form" aria-label={t("task.createTitle")} onSubmit={(e) => void submit(e)} noValidate>
      <Field
        label={t("task.title")}
        hint={t("task.titleHint")}
        error={titleError ? t(titleError) : undefined}
        value={title}
        maxLength={maxTitle}
        onChange={(e) => setTitle(e.target.value)}
      />
      <Field label={t("task.assignee")}>
        {({ id, describedBy }) => (
          <select id={id} aria-describedby={describedBy} value={assigneeId} onChange={(e) => setAssigneeId(e.target.value)}>
            <option value="">{t("task.unassigned")}</option>
            {(members.data ?? []).map((m) => (
              <option key={m.user_id} value={m.user_id}>
                {m.display_name || nameOf(m.user_id)}
              </option>
            ))}
          </select>
        )}
      </Field>
      <Field label={t("task.due")} type="date" value={due} onChange={(e) => setDue(e.target.value)} />
      <Field label={t("task.priority")}>
        {({ id, describedBy }) => (
          <select
            id={id}
            aria-describedby={describedBy}
            value={priority}
            onChange={(e) => setPriority(e.target.value as TaskPriority)}
          >
            {priorities.map((p) => (
              <option key={p} value={p}>
                {t(`task.priority.${p}`)}
              </option>
            ))}
          </select>
        )}
      </Field>
      {failed && (
        <p className="form-error" role="alert">
          {t(failed)}
        </p>
      )}
      <div className="task-form-actions">
        <button type="submit" disabled={saving}>
          {t("task.create")}
        </button>
        <button type="button" onClick={onDone}>
          {t("cancel")}
        </button>
      </div>
    </form>
  );
}
