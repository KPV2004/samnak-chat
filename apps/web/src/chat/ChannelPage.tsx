import { useEffect } from "react";
import { useParams, useSearchParams } from "react-router";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { ChannelHeader } from "./ChannelHeader";
import { Composer } from "./Composer";
import { MessageList } from "./MessageList";
import { ThreadPanel } from "./ThreadPanel";
import { TypingIndicator } from "./typing";

export function ChannelPage() {
  const t = useT();
  const { channelId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const rootId = params.get("thread");
  const chat = useChat();
  const { channels } = useChatState();
  const state = channels[channelId];
  const known = state !== undefined;
  const store = chat?.store;

  // Newest page of the history, the first time this channel is opened.
  useEffect(() => {
    if (store && known) void store.loadHistory(channelId).catch(() => {});
  }, [store, known, channelId]);

  // The chat area is full height, so it has no page column. `data-page` still drops the field margins inside it.
  if (!state) {
    if (!chat || chat.channelsStatus === "loading") {
      return (
        <div data-page className="grid h-full content-start gap-4 p-4">
          <p role="status" className="text-sm text-muted-foreground">
            {t("loading")}
          </p>
          {[0, 1, 2].map((i) => (
            <div key={i} className="flex gap-3">
              <Skeleton className="size-8 shrink-0 rounded-full" />
              <div className="grid flex-1 gap-2">
                <Skeleton className="h-3 w-28" />
                <Skeleton className="h-4 w-2/3" />
              </div>
            </div>
          ))}
        </div>
      );
    }
    return (
      <div data-page className="flex h-full items-center justify-center p-6">
        <p role="alert" className="text-center text-sm text-muted-foreground">
          {t("chat.notFound")}
        </p>
      </div>
    );
  }
  const channel = state.channel;
  return (
    <div data-page data-channel className="flex h-full min-w-0">
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <ChannelHeader channel={channel} />
        {/* The keys must differ from the composer's: both are children of this one array (BT-4). */}
        <MessageList key={`messages-${channelId}`} channelId={channelId} />
        <TypingIndicator channelId={channelId} />
        {channel.archived_at ? (
          <p className="mx-4 mb-4 rounded-lg border bg-muted/50 px-4 py-3 text-sm text-muted-foreground">{t("chat.archived")}</p>
        ) : (
          // Keyed by channel: opening another channel starts a new draft, with no picked names.
          <Composer key={`composer-${channelId}`} channelId={channelId} />
        )}
      </div>
      {rootId && (
        <ThreadPanel
          key={`${channelId}/${rootId}`}
          channelId={channelId}
          rootId={rootId}
          onClose={() =>
            setParams((prev) => {
              const next = new URLSearchParams(prev);
              next.delete("thread");
              return next;
            })
          }
        />
      )}
    </div>
  );
}
