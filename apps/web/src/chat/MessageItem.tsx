import type { ChatMessage } from "@samnak/core";
import { memo } from "react";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime, formatMessageTime } from "../lib/format";
import { useChat } from "../state/chat";
import { useNameOf } from "./names";

interface Props {
  message: ChatMessage;
  channelId: string;
}

export const MessageItem = memo(function MessageItem({ message: m, channelId }: Props) {
  const t = useT();
  const locale = useCurrentLocale();
  const chat = useChat();
  const nameOf = useNameOf();
  const mine = m.sender_id === chat?.userId;
  const deleted = m.deleted_at !== null;

  return (
    <li className={`chat-message${m.status === "pending" ? " pending" : ""}${m.status === "failed" ? " failed" : ""}`}>
      <div className="chat-message-head">
        <strong>{nameOf(m.sender_id)}</strong>{" "}
        {m.status === "sent" ? (
          <time dateTime={m.created_at} title={formatDateTime(m.created_at, locale)} className="muted">
            {formatMessageTime(m.created_at, locale)}
          </time>
        ) : null}
        {m.edited_at && !deleted && <span className="muted"> {t("chat.edited")}</span>}
      </div>
      {deleted ? <p className="muted chat-body">{t("chat.deleted")}</p> : <div className="chat-body">{m.body}</div>}
      {m.status === "pending" && <small className="muted">{t("chat.sending")}</small>}
      {m.status === "failed" && mine && <FailedActions channelId={channelId} clientMsgId={m.id} />}
    </li>
  );
});

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
