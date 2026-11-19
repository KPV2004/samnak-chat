import type { Channel, ChannelState } from "@samnak/core";
import { Hash, Lock, Plus, Search, SquarePen } from "lucide-react";
import { useMemo } from "react";
import { NavLink } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { NavItem, navItemClass } from "../components/NavItem";
import { SidebarSection } from "../components/SidebarSection";
import { initialOf } from "../components/UserAvatar";
import { useT } from "../i18n/useT";
import { useChat, useChatState } from "../state/chat";
import { cn } from "@/lib/utils";
import { channelTitle, useNameOf } from "./names";

const emptyClass = "m-0 px-2 py-1 text-sm text-muted-foreground";

/**
 * The channel and direct-message sections of the sidebar. AppSidebar shows them under the main links
 * on every page. Member names are only looked up (one request) once there is a direct message to title.
 */
export function Sidebar() {
  const t = useT();
  const chat = useChat();
  const { channels } = useChatState();

  const { rooms, dms } = useMemo(() => {
    const all = Object.values(channels);
    return {
      rooms: all.filter((s) => s.channel.type !== "dm").sort((a, b) => a.channel.name.localeCompare(b.channel.name)),
      dms: all.filter((s) => s.channel.type === "dm"),
    };
  }, [channels]);

  const status = chat?.channelsStatus ?? "loading";

  return (
    <nav aria-label={t("chat.sidebar")} className="grid gap-2">
      {status === "error" && (
        <div role="alert" className="grid gap-2 rounded-md border border-destructive/50 p-2 text-sm">
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
          <Skeleton className="h-5 w-4/5 bg-sidebar-accent" />
          <Skeleton className="h-5 w-3/5 bg-sidebar-accent" />
          <Skeleton className="h-5 w-2/3 bg-sidebar-accent" />
        </div>
      )}
      <SidebarSection id="channels" title={t("chat.channels")} badge={<UnreadTotal items={rooms} />}>
        <ul aria-label={t("chat.channels")} className="m-0 grid list-none gap-0.5 p-0">
          {rooms.map((state) => (
            <ChannelLink key={state.channel.id} state={state} title={state.channel.name} />
          ))}
        </ul>
        {status === "ready" && rooms.length === 0 && <p className={emptyClass}>{t("chat.noChannels")}</p>}
        <NavItem to="/chat/browse" icon={Search}>
          {t("chat.browse")}
        </NavItem>
        <NavItem to="/chat/new" icon={Plus}>
          {t("chat.newChannel")}
        </NavItem>
      </SidebarSection>
      <SidebarSection id="dms" title={t("chat.dms")} badge={<UnreadTotal items={dms} />}>
        {dms.length > 0 && <DmLinks dms={dms} />}
        {status === "ready" && dms.length === 0 && <p className={emptyClass}>{t("chat.noDms")}</p>}
        <NavItem to="/chat/dm" icon={SquarePen}>
          {t("chat.newDm")}
        </NavItem>
      </SidebarSection>
    </nav>
  );
}

function DmLinks({ dms }: { dms: ChannelState[] }) {
  const t = useT();
  const me = useChat()?.userId ?? "";
  const nameOf = useNameOf();
  const titled = useMemo(
    () => dms.map((state) => ({ state, title: channelTitle(state.channel, me, nameOf, t) })).sort((a, b) => a.title.localeCompare(b.title)),
    [dms, me, nameOf, t],
  );
  return (
    <ul aria-label={t("chat.dms")} className="m-0 grid list-none gap-0.5 p-0">
      {titled.map((c) => (
        <ChannelLink key={c.state.channel.id} state={c.state} title={c.title} />
      ))}
    </ul>
  );
}

/** What a folded section shows on its heading: the unread messages and mentions inside it. */
function UnreadTotal({ items }: { items: ChannelState[] }) {
  const t = useT();
  const unread = items.reduce((n, s) => n + s.unreadCount, 0);
  const mentions = items.reduce((n, s) => n + s.mentionCount, 0);
  if (unread === 0 && mentions === 0) return null;
  return mentions > 0 ? (
    <Badge variant="destructive" className="h-5 min-w-5 px-1.5">
      <span aria-hidden="true">@{mentions}</span>
      <span className="sr-only">{t("chat.mentions", { n: mentions })}</span>
    </Badge>
  ) : (
    <Badge className="h-5 min-w-5 px-1.5">
      <span aria-hidden="true">{unread}</span>
      <span className="sr-only">{t("chat.unread", { n: unread })}</span>
    </Badge>
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
        {channel.type === "dm" ? (
          <span aria-hidden="true" className="grid size-4 shrink-0 place-items-center rounded-sm bg-sidebar-foreground/20 text-[10px] leading-none font-semibold">
            {initialOf(title)}
          </span>
        ) : channel.type === "private" ? (
          <Lock aria-hidden="true" className="size-4 shrink-0 opacity-70" />
        ) : (
          <Hash aria-hidden="true" className="size-4 shrink-0 opacity-70" />
        )}
        <span className="min-w-0 flex-1 truncate">{title}</span>
        {state.unreadCount > 0 && (
          <Badge className="h-5 min-w-5 px-1.5">
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
