import { useEffect, useMemo, useRef } from "react";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { Composer } from "./Composer";
import { MessageItem } from "./MessageItem";

interface Props {
  channelId: string;
  rootId: string;
  onClose(): void;
}

/** A thread: the root message, its replies and a box to reply. Threads are one level deep. */
export function ThreadPanel({ channelId, rootId, onClose }: Props) {
  const t = useT();
  const store = useChat()?.store;
  const messages = useChatState().channels[channelId]?.messages;
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    void store?.loadThread(channelId, rootId).catch(() => {});
  }, [store, channelId, rootId]);

  const root = useMemo(() => messages?.find((m) => m.id === rootId && m.thread_root_id === null), [messages, rootId]);
  const replies = useMemo(() => (messages ?? []).filter((m) => m.thread_root_id === rootId), [messages, rootId]);

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [replies.length]);

  return (
    <aside className="chat-thread" aria-label={t("chat.thread.title")}>
      <header className="chat-thread-header">
        <h2>{t("chat.thread.title")}</h2>
        <button type="button" onClick={onClose}>
          {t("chat.thread.close")}
        </button>
      </header>
      <div ref={listRef} className="chat-thread-body">
        <ol>
          {root ? (
            <MessageItem message={root} channelId={channelId} inThread />
          ) : (
            <li className="muted">{t("chat.thread.rootMissing")}</li>
          )}
        </ol>
        {replies.length === 0 ? (
          <p className="muted">{t("chat.thread.empty")}</p>
        ) : (
          <ol>
            {replies.map((m) => (
              <MessageItem key={m.client_msg_id || m.id} message={m} channelId={channelId} inThread />
            ))}
          </ol>
        )}
      </div>
      <Composer channelId={channelId} threadRootId={rootId} label={t("chat.thread.reply")} />
    </aside>
  );
}
