import { ArrowDown, MessageSquareDashed } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { dayLabel, toRows } from "./grouping";
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
  const locale = useCurrentLocale();
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
  const rows = useMemo(() => toRows(topLevel), [topLevel]);
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
    <div className="relative flex min-h-0 flex-1 flex-col">
      <div
        ref={logRef}
        role="log"
        aria-label={t("chat.messages")}
        onScroll={onScroll}
        className="min-h-0 flex-1 overflow-y-auto px-2 pb-3 md:px-4"
      >
        {hasMore === true && (
          <div className="flex justify-center py-3">
            <Button type="button" variant="outline" size="sm" onClick={loadOlder}>
              {t("chat.loadOlder")}
            </Button>
          </div>
        )}
        {hasMore === false && <p className="py-3 text-center text-xs text-muted-foreground">{t("chat.startOfChannel")}</p>}
        {hasMore === false && topLevel.length === 0 && (
          <EmptyState icon={MessageSquareDashed} title={t("chat.noMessages")} className="mx-auto mt-4 max-w-sm" />
        )}
        {hasMore === null && <MessagesSkeleton label={t("loading")} />}
        <ol>
          {rows.map(({ message, continued, newDay }) => (
            <MessageItem
              key={message.client_msg_id || message.id}
              message={message}
              channelId={channelId}
              continued={continued}
              divider={newDay ? dayLabel(message.created_at, locale, t("chat.today")) : undefined}
            />
          ))}
        </ol>
      </div>
      {!atBottom && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="absolute bottom-3 left-1/2 -translate-x-1/2 rounded-full bg-background shadow-md"
          onClick={() => {
            const el = logRef.current;
            if (el) el.scrollTop = el.scrollHeight;
            atBottomRef.current = true;
            setAtBottom(true);
          }}
        >
          <ArrowDown />
          {t("chat.jumpLatest")}
        </Button>
      )}
    </div>
  );
}

/** Placeholder rows shaped like messages while the newest page loads. */
function MessagesSkeleton({ label }: { label: string }) {
  return (
    <div role="status" className="grid gap-5 px-2 py-4 md:px-3">
      <span className="sr-only">{label}</span>
      {[0, 1, 2].map((i) => (
        <div key={i} className="flex gap-3">
          <Skeleton className="size-8 shrink-0 rounded-full" />
          <div className="grid flex-1 gap-2">
            <Skeleton className="h-3 w-28" />
            <Skeleton className="h-4 w-3/4" />
          </div>
        </div>
      ))}
    </div>
  );
}
