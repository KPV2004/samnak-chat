import type { Channel, ChannelState } from "@samnak/core";
import { useMemo } from "react";
import { Link, NavLink } from "react-router";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { channelTitle, useNameOf } from "./names";

export function Sidebar() {
  const t = useT();
  const chat = useChat();
  const { channels } = useChatState();
  const nameOf = useNameOf();
  const me = chat?.userId ?? "";

  const { rooms, dms } = useMemo(() => {
    const titled = Object.values(channels).map((state) => ({ state, title: channelTitle(state.channel, me, nameOf, t) }));
    titled.sort((a, b) => a.title.localeCompare(b.title));
    return {
      rooms: titled.filter((c) => c.state.channel.type !== "dm"),
      dms: titled.filter((c) => c.state.channel.type === "dm"),
    };
  }, [channels, me, nameOf, t]);

  const status = chat?.channelsStatus ?? "loading";

  return (
    <nav className="chat-sidebar" aria-label={t("chat.sidebar")}>
      <div className="chat-sidebar-actions">
        <Link to="/chat/browse">{t("chat.browse")}</Link>
        <Link to="/chat/new">{t("chat.newChannel")}</Link>
        <Link to="/chat/dm">{t("chat.newDm")}</Link>
      </div>
      {status === "error" && (
        <div role="alert">
          <p>{t("chat.loadError")}</p>
          <button type="button" onClick={() => void chat?.reloadChannels()}>
            {t("retry")}
          </button>
        </div>
      )}
      {status === "loading" && rooms.length + dms.length === 0 && <p role="status">{t("loading")}</p>}
      <h2 id="chat-channels-heading">{t("chat.channels")}</h2>
      <ul aria-labelledby="chat-channels-heading">
        {rooms.map((c) => (
          <ChannelLink key={c.state.channel.id} state={c.state} title={c.title} />
        ))}
      </ul>
      {status === "ready" && rooms.length === 0 && <p className="muted">{t("chat.noChannels")}</p>}
      <h2 id="chat-dms-heading">{t("chat.dms")}</h2>
      <ul aria-labelledby="chat-dms-heading">
        {dms.map((c) => (
          <ChannelLink key={c.state.channel.id} state={c.state} title={c.title} />
        ))}
      </ul>
      {status === "ready" && dms.length === 0 && <p className="muted">{t("chat.noDms")}</p>}
    </nav>
  );
}

function ChannelLink({ state, title }: { state: ChannelState; title: string }) {
  const t = useT();
  const channel: Channel = state.channel;
  return (
    <li>
      <NavLink to={`/chat/${channel.id}`} className={channel.archived_at ? "archived" : undefined}>
        <span className="chat-link-name">
          {channel.type !== "dm" && <span aria-hidden="true"># </span>}
          {title}
        </span>
        {state.unreadCount > 0 && (
          <span className="badge unread">
            <span aria-hidden="true">{state.unreadCount}</span>
            <span className="visually-hidden">{t("chat.unread", { n: state.unreadCount })}</span>
          </span>
        )}
        {state.mentionCount > 0 && (
          <span className="badge mention">
            <span aria-hidden="true">@{state.mentionCount}</span>
            <span className="visually-hidden">{t("chat.mentions", { n: state.mentionCount })}</span>
          </span>
        )}
      </NavLink>
    </li>
  );
}
