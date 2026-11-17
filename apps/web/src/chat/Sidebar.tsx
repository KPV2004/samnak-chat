import type { Channel, ChannelState } from "@samnak/core";
import { Hash, Plus, Search, SquarePen } from "lucide-react";
import { useMemo } from "react";
import { NavLink } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { NavItem, navItemClass } from "../components/NavItem";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { cn } from "@/lib/utils";
import { channelTitle, useNameOf } from "./names";

const headingClass = "m-0 px-2 pb-1 text-xs font-medium text-muted-foreground";

/** The channel and direct-message list. AppSidebar shows it under the main links while chat is open. */
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
    <nav aria-label={t("chat.sidebar")} className="grid gap-4">
      {status === "error" && (
        <div role="alert" className="grid gap-2 rounded-md border border-destructive/40 p-2 text-sm">
          <p className="m-0 text-destructive">{t("chat.loadError")}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void chat?.reloadChannels()}>
            {t("retry")}
          </Button>
        </div>
      )}
      {status === "loading" && rooms.length + dms.length === 0 && (
        <div className="grid gap-1.5 px-2">
          <p role="status" className="sr-only">
            {t("loading")}
          </p>
          <Skeleton className="h-5 w-4/5" />
          <Skeleton className="h-5 w-3/5" />
          <Skeleton className="h-5 w-2/3" />
        </div>
      )}
      <section>
        <h2 id="chat-channels-heading" className={headingClass}>
          {t("chat.channels")}
        </h2>
        <ul aria-labelledby="chat-channels-heading" className="m-0 grid list-none gap-0.5 p-0">
          {rooms.map((c) => (
            <ChannelLink key={c.state.channel.id} state={c.state} title={c.title} />
          ))}
        </ul>
        {status === "ready" && rooms.length === 0 && <p className="m-0 px-2 text-sm text-muted-foreground">{t("chat.noChannels")}</p>}
        <div className="mt-0.5 grid gap-0.5">
          <NavItem to="/chat/browse" icon={Search}>
            {t("chat.browse")}
          </NavItem>
          <NavItem to="/chat/new" icon={Plus}>
            {t("chat.newChannel")}
          </NavItem>
        </div>
      </section>
      <section>
        <h2 id="chat-dms-heading" className={headingClass}>
          {t("chat.dms")}
        </h2>
        <ul aria-labelledby="chat-dms-heading" className="m-0 grid list-none gap-0.5 p-0">
          {dms.map((c) => (
            <ChannelLink key={c.state.channel.id} state={c.state} title={c.title} />
          ))}
        </ul>
        {status === "ready" && dms.length === 0 && <p className="m-0 px-2 text-sm text-muted-foreground">{t("chat.noDms")}</p>}
        <div className="mt-0.5 grid gap-0.5">
          <NavItem to="/chat/dm" icon={SquarePen}>
            {t("chat.newDm")}
          </NavItem>
        </div>
      </section>
    </nav>
  );
}

function ChannelLink({ state, title }: { state: ChannelState; title: string }) {
  const t = useT();
  const channel: Channel = state.channel;
  const unread = state.unreadCount > 0 || state.mentionCount > 0;
  return (
    <li>
      <NavLink
        to={`/chat/${channel.id}`}
        className={cn(navItemClass, unread && "font-semibold text-foreground", channel.archived_at && "opacity-60")}
      >
        {channel.type !== "dm" && <Hash aria-hidden="true" className="size-4 shrink-0 opacity-60" />}
        <span className="min-w-0 flex-1 truncate">{title}</span>
        {state.unreadCount > 0 && (
          <Badge variant="secondary" className="h-5 min-w-5 px-1.5">
            <span aria-hidden="true">{state.unreadCount}</span>
            <span className="sr-only">{t("chat.unread", { n: state.unreadCount })}</span>
          </Badge>
        )}
        {state.mentionCount > 0 && (
          <Badge variant="destructive" className="h-5 min-w-5 px-1.5">
            <span aria-hidden="true">@{state.mentionCount}</span>
            <span className="sr-only">{t("chat.mentions", { n: state.mentionCount })}</span>
          </Badge>
        )}
      </NavLink>
    </li>
  );
}
