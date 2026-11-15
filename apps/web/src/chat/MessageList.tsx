import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { MessageItem } from "./MessageItem";

/** How close to the bottom still counts as "the newest message is visible", in pixels. */
const nearBottom = 48;

function usePageVisible(): boolean {
  const [visible, setVisible] = useState(() => document.visibilityState !== "hidden");
  useEffect(() => {
    const onChange = () => setVisible(document.visibilityState !== "hidden");
    document.addEventListener("visibilitychange", onChange);
    return () => document.removeEventListener("visibilitychange", onChange);
  }, []);
  return visible;
}

export function MessageList({ channelId }: { channelId: string }) {
  const t = useT();
  const chat = useChat();
  const state = useChatState().channels[channelId];
  const logRef = useRef<HTMLDivElement>(null);
  const [atBottom, setAtBottom] = useState(true);
  const atBottomRef = useRef(true);
  const visible = usePageVisible();
  // Set when older messages are requested, so the view keeps its place when they arrive above.
  const anchor = useRef<{ height: number; top: number } | null>(null);
  const store = chat?.store;
  const me = chat?.userId;

  // The open channel counts new messages as read while its newest one is in view.
  const reading = atBottom && visible;
  useEffect(() => {
    if (!store) return;
    void store.setActiveChannel(reading ? channelId : null);
    return () => {
      void store.setActiveChannel(null);
    };
  }, [store, channelId, reading]);

  const topLevel = useMemo(() => (state?.messages ?? []).filter((m) => m.thread_root_id === null), [state?.messages]);
  const newest = topLevel.at(-1);
  const newestKey = newest ? newest.client_msg_id || newest.id : null;
  const newestIsMine = newest?.sender_id === me;

  // Older messages arrived above: keep what the reader was looking at in place.
  const oldestKey = topLevel[0]?.id;
  useLayoutEffect(() => {
    const el = logRef.current;
    if (el && anchor.current) {
      el.scrollTop = anchor.current.top + (el.scrollHeight - anchor.current.height);
      anchor.current = null;
    }
  }, [oldestKey]);

  // A new newest message follows the reader down, unless they scrolled up (their own message always does).
  useLayoutEffect(() => {
    const el = logRef.current;
    if (el && (atBottomRef.current || newestIsMine)) el.scrollTop = el.scrollHeight;
  }, [newestKey, newestIsMine]);

  const onScroll = useCallback(() => {
    const el = logRef.current;
    if (!el) return;
    const bottom = el.scrollHeight - el.scrollTop - el.clientHeight <= nearBottom;
    atBottomRef.current = bottom;
    setAtBottom(bottom);
  }, []);

  function loadOlder() {
    const el = logRef.current;
    if (el) anchor.current = { height: el.scrollHeight, top: el.scrollTop };
    void store?.loadHistory(channelId).catch(() => {
      anchor.current = null;
    });
  }

  if (!state) return null;
  const hasMore = state.hasMoreBefore;

  return (
    <div className="chat-log-wrap">
      <div ref={logRef} className="chat-log" role="log" aria-label={t("chat.messages")} onScroll={onScroll}>
        {hasMore === true && (
          <button type="button" className="chat-load-older" onClick={loadOlder}>
            {t("chat.loadOlder")}
          </button>
        )}
        {hasMore === false && <p className="muted chat-start">{t("chat.startOfChannel")}</p>}
        {hasMore === false && topLevel.length === 0 && <p className="muted">{t("chat.noMessages")}</p>}
        {hasMore === null && <p role="status">{t("loading")}</p>}
        <ol>
          {topLevel.map((m) => (
            <MessageItem key={m.client_msg_id || m.id} message={m} channelId={channelId} />
          ))}
        </ol>
      </div>
      {!atBottom && (
        <button
          type="button"
          className="chat-jump"
          onClick={() => {
            const el = logRef.current;
            if (el) el.scrollTop = el.scrollHeight;
            atBottomRef.current = true;
            setAtBottom(true);
          }}
        >
          {t("chat.jumpLatest")}
        </button>
      )}
    </div>
  );
}
