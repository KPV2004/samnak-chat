import { sortConversations, type Channel, type ChannelState } from "@samnak/core";
import { BellOff, EllipsisVertical, Search, SquarePen } from "lucide-react";
import { useMemo, useState } from "react";
import { Link, NavLink } from "react-router";
import { Button } from "@/components/ui/button";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from "@/components/ui/context-menu";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { navItemClass } from "../components/NavItem";
import { useT, type T } from "../i18n/useT";
import { useLocale } from "../state/locale";
import { MuteMenuItem } from "../notify/ChannelMuteMenu";
import { useChannelMute } from "../notify/usePreferences";
import { statusKeys as approvalStatusKeys } from "../approval/format";
import { useChat, useChatState } from "../state/chat";
import { ConversationAvatar } from "./ConversationAvatar";
import { formatListTime } from "./format";
import { channelTitle, useNameOf } from "./names";
import { PinMenuItem, type MenuItem } from "./pins";
import { UnreadBadges } from "./UnreadBadges";

type Filter = "all" | "unread" | "groups" | "direct";
const filters: Filter[] = ["all", "unread", "groups", "direct"];

/** A 1:1 message (or one with yourself); every other conversation is a group. */
function isDirect(channel: Channel, me: string): boolean {
  return channel.type === "dm" && channel.dm_user_ids.filter((id) => id !== me).length <= 1;
}

function matches(filter: Filter, state: ChannelState, me: string): boolean {
  switch (filter) {
    case "all":
      return true;
    case "unread":
      return state.unreadCount > 0 || state.mentionCount > 0;
    case "direct":
      return isDirect(state.channel, me);
    case "groups":
      return !isDirect(state.channel, me);
  }
}

interface Row {
  state: ChannelState;
  title: string;
}

/**
 * The sidebar of Messenger, as in Lark: a search box, filter chips, the pinned conversations, then
 * every conversation by its latest message. Each row has a picture, the title, `who: text`, the
 * time, the unread badge and a mute mark; hover or focus shows an options button (Pin, Mute), and a
 * right-click opens the same menu.
 */
export function ConversationList() {
  const t = useT();
  const chat = useChat();
  const me = chat?.userId ?? "";
  const nameOf = useNameOf();
  const { channels } = useChatState();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const status = chat?.channelsStatus ?? "loading";

  const { all, shown } = useMemo(() => {
    const titles = new Map(Object.values(channels).map((s) => [s.channel.id, channelTitle(s.channel, me, nameOf, t)]));
    const sorted = sortConversations(Object.values(channels), (s) => titles.get(s.channel.id) ?? "");
    const rows: Row[] = sorted.map((state) => ({ state, title: titles.get(state.channel.id) ?? "" }));
    const needle = query.trim().toLocaleLowerCase();
    return {
      all: rows,
      shown: rows.filter((r) => matches(filter, r.state, me) && (!needle || r.title.toLocaleLowerCase().includes(needle))),
    };
  }, [channels, me, nameOf, t, query, filter]);

  const pinned = shown.filter((r) => r.state.channel.pinned_at !== null);
  const recent = shown.filter((r) => r.state.channel.pinned_at === null);

  return (
    <nav aria-label={t("chat.sidebar")} className="flex min-h-0 flex-1 flex-col gap-2">
      <div className="grid shrink-0 gap-2 px-2">
        <div className="flex h-8 items-center gap-1">
          <h2 className="m-0 min-w-0 flex-1 truncate px-1 text-base font-semibold">{t("chat.list.title")}</h2>
          <NewChatMenu />
        </div>
        <div role="search" className="relative">
          <Search aria-hidden="true" className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            type="search"
            aria-label={t("chat.list.search")}
            placeholder={t("chat.list.searchPlaceholder")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            className="h-8 pl-8"
          />
        </div>
        <div role="group" aria-label={t("chat.filter.label")} className="flex flex-wrap gap-1">
          {filters.map((f) => (
            <Button
              key={f}
              type="button"
              variant="ghost"
              size="sm"
              aria-pressed={filter === f}
              className={cn(
                "h-7 rounded-sm px-2.5 text-xs",
                filter === f && "bg-sidebar-primary text-sidebar-primary-foreground hover:bg-sidebar-primary/90 hover:text-sidebar-primary-foreground",
              )}
              onClick={() => setFilter(f)}
            >
              {t(`chat.filter.${f}`)}
            </Button>
          ))}
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2">
        {status === "error" && (
          <div role="alert" className="grid gap-2 rounded-md border border-destructive/50 p-2 text-sm">
            <p className="m-0 text-destructive">{t("chat.loadError")}</p>
            <Button type="button" variant="outline" size="sm" onClick={() => void chat?.reloadChannels()}>
              {t("retry")}
            </Button>
          </div>
        )}
        {status === "loading" && all.length === 0 && (
          <div className="grid gap-3 px-1 py-1">
            <p role="status" className="sr-only">
              {t("loading")}
            </p>
            {[0, 1, 2].map((n) => (
              <div key={n} className="flex items-center gap-2.5">
                <Skeleton className="size-10 shrink-0 rounded-full bg-sidebar-accent" />
                <div className="grid flex-1 gap-1.5">
                  <Skeleton className="h-3.5 w-2/3 bg-sidebar-accent" />
                  <Skeleton className="h-3 w-full bg-sidebar-accent" />
                </div>
              </div>
            ))}
          </div>
        )}
        {status === "ready" && all.length === 0 && <p className="m-0 px-2 py-1 text-sm text-muted-foreground">{t("chat.noChannels")}</p>}
        {all.length > 0 && shown.length === 0 && <p className="m-0 px-2 py-1 text-sm text-muted-foreground">{t("chat.list.noMatch")}</p>}
        {pinned.length > 0 && (
          <section className="grid gap-0.5">
            <h3 className="m-0 px-2 pt-1 text-xs font-medium text-muted-foreground">{t("chat.pinned")}</h3>
            <ConversationRows label={t("chat.pinned")} rows={pinned} />
          </section>
        )}
        {recent.length > 0 && (
          <section className="grid gap-0.5">
            {pinned.length > 0 && <h3 className="m-0 px-2 pt-2 text-xs font-medium text-muted-foreground">{t("chat.recent")}</h3>}
            <ConversationRows label={t("chat.recent")} rows={recent} />
          </section>
        )}
      </div>
    </nav>
  );
}

/** The button of the list's header: a 1:1 or group message, a channel, or the channels to join. */
function NewChatMenu() {
  const t = useT();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button type="button" variant="ghost" size="icon-sm" aria-label={t("chat.list.new")} title={t("chat.list.new")}>
          <SquarePen aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem asChild>
          <Link to="/chat/dm">{t("chat.newDm")}</Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/chat/new">{t("chat.newChannel")}</Link>
        </DropdownMenuItem>
        <DropdownMenuItem asChild>
          <Link to="/chat/browse">{t("chat.browse")}</Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ConversationRows({ label, rows }: { label: string; rows: Row[] }) {
  return (
    <ul aria-label={label} className="m-0 grid list-none gap-0.5 p-0">
      {rows.map((r) => (
        <ConversationRow key={r.state.channel.id} state={r.state} title={r.title} />
      ))}
    </ul>
  );
}

/** What the menus of a conversation offer; only built while a menu is open. */
function MenuEntries({ channel, mute, Item }: { channel: Channel; mute: ReturnType<typeof useChannelMute>; Item: MenuItem }) {
  return (
    <>
      <PinMenuItem channel={channel} Item={Item} />
      <MuteMenuItem mute={mute} Item={Item} />
    </>
  );
}

/** A card's preview is written by the server in English; the web says the same in the reader's language (BT-5). */
function previewText(kind: string, preview: string, t: T): string {
  if (kind === "approval.request") {
    const matched = /^Approval request: (.*) \((pending|approved|rejected|cancelled)\)$/.exec(preview);
    if (matched) {
      const status = matched[2] as keyof typeof approvalStatusKeys;
      return t("chat.preview.approval", { summary: matched[1]!, status: t(approvalStatusKeys[status]) });
    }
  }
  if (kind === "task.item" && preview.startsWith("Task: ")) {
    return t("chat.preview.task", { title: preview.slice("Task: ".length) });
  }
  return preview || t("chat.preview.empty");
}

/** The line under a title: `who: text`, or only the text in a 1:1 message, where the title already says who. */
function previewLine(channel: Channel, me: string, nameOf: (id: string) => string, t: T): string {
  const last = channel.last_message;
  if (!last) return t("chat.preview.none");
  const text = previewText(last.kind, last.preview, t);
  if (last.sender_id === me) return t("chat.preview.from", { name: t("chat.preview.you"), text });
  return isDirect(channel, me) ? text : t("chat.preview.from", { name: nameOf(last.sender_id), text });
}

function ConversationRow({ state, title }: { state: ChannelState; title: string }) {
  const t = useT();
  const locale = useLocale((s) => s.locale);
  const me = useChat()?.userId ?? "";
  const nameOf = useNameOf();
  const channel = state.channel;
  // Here, not in the menus: they unmount when an entry is chosen, and the request's toast must outlive them.
  const mute = useChannelMute(channel.id);
  const { muted } = mute;
  const unread = state.unreadCount > 0 || state.mentionCount > 0;
  const last = channel.last_message;

  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>
        <li className="group/row relative">
          <NavLink
            to={`/chat/${channel.id}`}
            className={cn(navItemClass, "group/link h-auto gap-2.5 py-2 md:h-auto", channel.archived_at && "opacity-60")}
          >
            <ConversationAvatar channel={channel} title={title} />
            <span className="grid min-w-0 flex-1 grid-cols-[minmax(0,1fr)] gap-0.5 leading-snug">
              <span className="flex items-center gap-1.5">
                <span data-title className={cn("min-w-0 flex-1 truncate text-sm", unread ? "font-semibold text-foreground" : "font-medium")}>
                  {title}
                </span>
                {muted && (
                  <>
                    <BellOff aria-hidden="true" className="size-3.5 shrink-0 opacity-70" />
                    <span className="sr-only">{t("chat.muted")}</span>
                  </>
                )}
                {last && (
                  <time
                    dateTime={last.created_at}
                    className="shrink-0 text-xs font-normal text-muted-foreground group-aria-[current=page]/link:text-sidebar-primary-foreground/70 group-has-[button:focus-visible]/row:opacity-0 group-has-[button[data-state=open]]/row:opacity-0 group-hover/row:opacity-0"
                  >
                    {formatListTime(last.created_at, locale, t("chat.time.yesterday"))}
                  </time>
                )}
              </span>
              <span className="flex items-center gap-1.5">
                <span
                  className={cn(
                    "min-w-0 flex-1 truncate text-xs font-normal text-muted-foreground group-aria-[current=page]/link:text-sidebar-primary-foreground/70",
                    unread && "text-foreground",
                  )}
                >
                  {previewLine(channel, me, nameOf, t)}
                </span>
                <UnreadBadges state={state} />
              </span>
            </span>
          </NavLink>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label={t("chat.rowMenu", { name: title })}
                className="absolute top-1.5 right-1.5 size-6 opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 data-[state=open]:opacity-100"
              >
                <EllipsisVertical aria-hidden="true" />
              </Button>
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start">
              <MenuEntries channel={channel} mute={mute} Item={DropdownMenuItem} />
            </DropdownMenuContent>
          </DropdownMenu>
        </li>
      </ContextMenuTrigger>
      <ContextMenuContent>
        <MenuEntries channel={channel} mute={mute} Item={ContextMenuItem} />
      </ContextMenuContent>
    </ContextMenu>
  );
}
