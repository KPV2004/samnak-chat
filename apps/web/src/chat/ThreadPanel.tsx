import { X } from "lucide-react";
import { useEffect, useMemo, useRef, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { Composer } from "./Composer";
import { toRows } from "./grouping";
import { MessageItem } from "./MessageItem";
import { useWide } from "./useWide";

interface Props {
  channelId: string;
  rootId: string;
  onClose(): void;
}

/**
 * A thread: the root message, its replies and a box to reply. Threads are one level deep.
 * On a wide screen it is a side panel; on a phone it is a Sheet over the channel.
 */
export function ThreadPanel({ channelId, rootId, onClose }: Props) {
  const t = useT();
  const store = useChat()?.store;
  const wide = useWide();
  const messages = useChatState().channels[channelId]?.messages;
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    void store?.loadThread(channelId, rootId).catch(() => {});
  }, [store, channelId, rootId]);

  const root = useMemo(() => messages?.find((m) => m.id === rootId && m.thread_root_id === null), [messages, rootId]);
  const replies = useMemo(() => (messages ?? []).filter((m) => m.thread_root_id === rootId), [messages, rootId]);
  const rows = useMemo(() => toRows(replies), [replies]);

  useEffect(() => {
    const el = listRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [replies.length]);

  const header = (title: ReactNode) => (
    <header className="flex h-12 shrink-0 items-center gap-2 border-b px-4">
      <div className="min-w-0 flex-1">{title}</div>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button type="button" variant="ghost" size="icon-sm" aria-label={t("chat.thread.close")} onClick={onClose}>
            <X />
          </Button>
        </TooltipTrigger>
        <TooltipContent>{t("chat.thread.close")}</TooltipContent>
      </Tooltip>
    </header>
  );

  const body = (
    <>
      <div ref={listRef} className="min-h-0 flex-1 overflow-y-auto px-2 py-3 md:px-3">
        <ol>
          {root ? (
            <MessageItem message={root} channelId={channelId} inThread />
          ) : (
            <li className="px-2 text-sm text-muted-foreground">{t("chat.thread.rootMissing")}</li>
          )}
        </ol>
        {replies.length === 0 ? (
          <p className="px-3 pt-3 text-sm text-muted-foreground">{t("chat.thread.empty")}</p>
        ) : (
          <ol className="mt-2 border-t pt-2">
            {rows.map(({ message, continued }) => (
              <MessageItem key={message.client_msg_id || message.id} message={message} channelId={channelId} inThread continued={continued} />
            ))}
          </ol>
        )}
      </div>
      <Composer channelId={channelId} threadRootId={rootId} label={t("chat.thread.reply")} />
    </>
  );

  if (wide) {
    return (
      <aside aria-label={t("chat.thread.title")} className="flex min-h-0 w-96 shrink-0 flex-col border-l bg-background">
        {header(<h2 className="m-0 text-sm font-semibold">{t("chat.thread.title")}</h2>)}
        {body}
      </aside>
    );
  }

  return (
    <Sheet
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <SheetContent side="right" showCloseButton={false} className="flex w-full flex-col gap-0 p-0 sm:max-w-md">
        {header(
          <SheetTitle asChild>
            <h2 className="m-0 text-sm font-semibold">{t("chat.thread.title")}</h2>
          </SheetTitle>,
        )}
        {body}
      </SheetContent>
    </Sheet>
  );
}
