import type { ChatMessage, Reaction } from "@samnak/core";
import { memo, useState } from "react";
import { useSearchParams } from "react-router";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime, formatMessageTime } from "../lib/format";
import { useChat } from "../state/chat";
import { canManage, useWorkspaces } from "../state/queries";
import { useChatExtensions } from "../task/api";
import { CreateTaskForm } from "../task/CreateTaskForm";
import { CardBody } from "./cards";
import { EditForm } from "./EditForm";
import { Markdown } from "./markdown";
import { useNameOf } from "./names";

const quickReactions = ["👍", "❤️", "😂", "🎉", "🙏", "👀"];

interface Props {
  message: ChatMessage;
  channelId: string;
  /** Inside the thread panel: no reply button, and the replies are not counted again. */
  inThread?: boolean;
}

export const MessageItem = memo(function MessageItem({ message: m, channelId, inThread = false }: Props) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  const role = useWorkspaces().current?.role;
  const [, setParams] = useSearchParams();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [picking, setPicking] = useState(false);
  const [creatingTask, setCreatingTask] = useState(false);
  const [failed, setFailed] = useState(false);
  const { messageActions } = useChatExtensions();
  const canCreateTask = messageActions.some((a) => a.id === "task.create_from_message");

  const me = chat?.userId ?? "";
  const mine = m.sender_id === me;
  const deleted = m.deleted_at !== null;
  const sent = m.status === "sent";
  const canDelete = sent && !deleted && (mine || canManage(role));

  function openThread() {
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set("thread", m.id);
      return next;
    });
  }

  async function remove() {
    if (!chat) return;
    setFailed(false);
    try {
      await chat.api.deleteMessage(m.id);
      chat.store.applyMessage({ ...m, body: "", deleted_at: new Date().toISOString() });
      setConfirming(false);
    } catch {
      setFailed(true);
    }
  }

  /** Changes my reaction at once, and puts the message back if the server refuses. */
  async function react(emoji: string) {
    if (!chat) return;
    const had = m.reactions.find((r) => r.emoji === emoji);
    const adding = !had?.me;
    chat.store.applyMessage({ ...m, reactions: nextReactions(m.reactions, emoji, adding) });
    setPicking(false);
    setFailed(false);
    try {
      if (adding) await chat.api.addReaction(m.id, emoji);
      else await chat.api.removeReaction(m.id, emoji);
    } catch {
      chat.store.applyMessage(m);
      setFailed(true);
    }
  }

  return (
    <li className={`chat-message${m.status === "pending" ? " pending" : ""}${m.status === "failed" ? " failed" : ""}`}>
      <div className="chat-message-head">
        <strong>{nameOf(m.sender_id)}</strong>{" "}
        {sent && (
          <time dateTime={m.created_at} title={formatDateTime(m.created_at, locale)} className="muted">
            {formatMessageTime(m.created_at, locale)}
          </time>
        )}
        {m.edited_at && !deleted && <span className="muted"> {t("chat.edited")}</span>}
      </div>

      {deleted ? (
        <p className="muted chat-body">{t("chat.deleted")}</p>
      ) : editing ? (
        <EditForm message={m} onDone={() => setEditing(false)} />
      ) : m.kind !== "text" ? (
        <CardBody message={m} />
      ) : (
        m.body !== "" && (
          <div className="chat-body">
            <Markdown text={m.body} nameOf={nameOf} me={me} />
          </div>
        )
      )}

      {!deleted && m.attachments.length > 0 && (
        <ul className="chat-attachments" aria-label={t("chat.attachments")}>
          {m.attachments.map((a) => (
            <li key={a.id}>{a.file_name}</li>
          ))}
        </ul>
      )}

      {!deleted && m.reactions.length > 0 && (
        <ul className="chat-reactions">
          {m.reactions.map((r) => (
            <li key={r.emoji}>
              <button
                type="button"
                aria-pressed={r.me}
                aria-label={t("chat.reactionLabel", { emoji: r.emoji, n: r.count })}
                onClick={() => void react(r.emoji)}
              >
                {t("chat.reactionToggle", { emoji: r.emoji, n: r.count })}
              </button>
            </li>
          ))}
        </ul>
      )}

      {!inThread && m.reply_count > 0 && !deleted && (
        <button type="button" className="chat-replies" onClick={openThread}>
          {m.reply_count === 1 ? t("chat.replyCount.one") : t("chat.replyCount.other", { n: m.reply_count })}
        </button>
      )}

      {m.status === "pending" && <small className="muted">{t("chat.sending")}</small>}
      {m.status === "failed" && mine && <FailedActions channelId={channelId} clientMsgId={m.id} />}

      {confirming && (
        <div className="chat-confirm" role="group">
          <span>{t("chat.deleteConfirm")}</span>
          <button type="button" className="danger" onClick={() => void remove()}>
            {t("confirm")}
          </button>
          <button type="button" onClick={() => setConfirming(false)}>
            {t("cancel")}
          </button>
        </div>
      )}

      {sent && !deleted && !editing && !confirming && (
        <div className="chat-actions" role="group">
          <button type="button" aria-expanded={picking} onClick={() => setPicking((open) => !open)}>
            {t("chat.react")}
          </button>
          {!inThread && m.thread_root_id === null && (
            <button type="button" onClick={openThread}>
              {t("chat.reply")}
            </button>
          )}
          {canCreateTask && m.kind === "text" && m.body !== "" && (
            <button type="button" aria-expanded={creatingTask} onClick={() => setCreatingTask((open) => !open)}>
              {t("task.createFromMessage")}
            </button>
          )}
          {mine && (
            <button type="button" onClick={() => setEditing(true)}>
              {t("chat.edit")}
            </button>
          )}
          {canDelete && (
            <button type="button" onClick={() => setConfirming(true)}>
              {t("chat.delete")}
            </button>
          )}
        </div>
      )}

      {creatingTask && <CreateTaskForm message={m} channelId={channelId} onDone={() => setCreatingTask(false)} />}

      {picking && (
        <ul className="chat-emoji-picker">
          {quickReactions.map((emoji) => (
            <li key={emoji}>
              <button type="button" onClick={() => void react(emoji)}>
                {emoji}
              </button>
            </li>
          ))}
        </ul>
      )}

      {failed && (
        <p className="form-error" role="alert">
          {t("chat.actionError")}
        </p>
      )}
    </li>
  );
});

function nextReactions(reactions: Reaction[], emoji: string, adding: boolean): Reaction[] {
  const found = reactions.some((r) => r.emoji === emoji);
  if (adding && !found) return [...reactions, { emoji, count: 1, me: true }];
  return reactions
    .map((r) => (r.emoji === emoji ? { ...r, count: r.count + (adding ? 1 : -1), me: adding } : r))
    .filter((r) => r.count > 0);
}

function FailedActions({ channelId, clientMsgId }: { channelId: string; clientMsgId: string }) {
  const t = useT();
  const store = useChat()?.store;
  return (
    <div className="chat-failed" role="group">
      <span className="form-error">{t("chat.failed")}</span>
      <button type="button" onClick={() => void store?.retry(channelId, clientMsgId)}>
        {t("retry")}
      </button>
      <button type="button" onClick={() => store?.discard(channelId, clientMsgId)}>
        {t("chat.discard")}
      </button>
    </div>
  );
}
