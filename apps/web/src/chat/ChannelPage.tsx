import { useParams } from "react-router";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { channelTitle, useNameOf } from "./names";

export function ChannelPage() {
  const t = useT();
  const { channelId = "" } = useParams();
  const chat = useChat();
  const { channels } = useChatState();
  const nameOf = useNameOf();
  const state = channels[channelId];

  if (!state) {
    if (!chat || chat.channelsStatus === "loading") return <p role="status">{t("loading")}</p>;
    return <p role="alert">{t("chat.notFound")}</p>;
  }
  const channel = state.channel;
  return (
    <div className="chat-channel">
      <header className="chat-channel-header">
        <h1>{channelTitle(channel, chat?.userId ?? "", nameOf, t)}</h1>
        {channel.topic && <p className="muted">{channel.topic}</p>}
      </header>
    </div>
  );
}
