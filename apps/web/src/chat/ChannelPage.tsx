import { useEffect } from "react";
import { useParams, useSearchParams } from "react-router";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { Composer } from "./Composer";
import { MessageList } from "./MessageList";
import { ThreadPanel } from "./ThreadPanel";
import { TypingIndicator } from "./typing";
import { channelTitle, useNameOf } from "./names";

export function ChannelPage() {
  const t = useT();
  const { channelId = "" } = useParams();
  const [params, setParams] = useSearchParams();
  const rootId = params.get("thread");
  const chat = useChat();
  const { channels } = useChatState();
  const nameOf = useNameOf();
  const state = channels[channelId];
  const known = state !== undefined;
  const store = chat?.store;

  // Newest page of the history, the first time this channel is opened.
  useEffect(() => {
    if (store && known) void store.loadHistory(channelId).catch(() => {});
  }, [store, known, channelId]);

  if (!state) {
    if (!chat || chat.channelsStatus === "loading") return <p role="status">{t("loading")}</p>;
    return <p role="alert">{t("chat.notFound")}</p>;
  }
  const channel = state.channel;
  return (
    <div className="chat-channel">
      <div className="chat-main">
        <header className="chat-channel-header">
          <h1>{channelTitle(channel, chat?.userId ?? "", nameOf, t)}</h1>
          {channel.topic && <p className="muted">{channel.topic}</p>}
          <small className="muted">{t("chat.members", { n: channel.member_count })}</small>
        </header>
        <MessageList channelId={channelId} />
        <TypingIndicator channelId={channelId} />
        {channel.archived_at ? (
          <p className="chat-archived muted">{t("chat.archived")}</p>
        ) : (
          <Composer channelId={channelId} />
        )}
      </div>
      {rootId && (
        <ThreadPanel
          key={rootId}
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
