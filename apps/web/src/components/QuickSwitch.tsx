import { Hash, Lock, MessagesSquare, Search } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router";
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { cn } from "@/lib/utils";
import { channelTitle, useNameOf } from "../chat/names";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { useChat, useChatState } from "../state/chat";
import { canManage, useWorkspaces } from "../state/queries";

interface Target {
  to: string;
  title: string;
}

/**
 * The search-style bar in the top bar. It names the workspace and opens a "jump to" list of its
 * channels, direct messages and pages (also on Ctrl or Cmd + K). It finds places, not message text.
 */
/** The channel and direct-message groups. Mounted with the open dialog, so the member lookup for names runs only then. */
function ChannelGroups({ onPick }: { onPick: (to: string) => void }) {
  const t = useT();
  const chat = useChat();
  const { channels } = useChatState();
  const nameOf = useNameOf();
  const me = chat?.userId ?? "";
  const { rooms, dms } = useMemo(() => {
    const titled = Object.values(channels)
      .filter((s) => !s.channel.archived_at)
      .map((s) => ({ channel: s.channel, title: channelTitle(s.channel, me, nameOf, t) }));
    titled.sort((a, b) => a.title.localeCompare(b.title));
    return { rooms: titled.filter((c) => c.channel.type !== "dm"), dms: titled.filter((c) => c.channel.type === "dm") };
  }, [channels, me, nameOf, t]);

  return (
    <>
      {rooms.length > 0 && (
        <CommandGroup heading={t("chat.channels")}>
          {rooms.map((c) => (
            <CommandItem key={c.channel.id} value={`${c.title} ${c.channel.id}`} onSelect={() => onPick(`/chat/${c.channel.id}`)}>
              {c.channel.type === "private" ? <Lock aria-hidden="true" /> : <Hash aria-hidden="true" />}
              {c.title}
            </CommandItem>
          ))}
        </CommandGroup>
      )}
      {dms.length > 0 && (
        <CommandGroup heading={t("chat.dms")}>
          {dms.map((c) => (
            <CommandItem key={c.channel.id} value={`${c.title} ${c.channel.id}`} onSelect={() => onPick(`/chat/${c.channel.id}`)}>
              <MessagesSquare aria-hidden="true" />
              {c.title}
            </CommandItem>
          ))}
        </CommandGroup>
      )}
    </>
  );
}

export function QuickSwitch({ className }: { className?: string }) {
  const t = useT();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const { current } = useWorkspaces();
  const taskOn = useModuleEnabled("task");
  const approvalOn = useModuleEnabled("approval");
  const helpdeskOn = useModuleEnabled("helpdesk");

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setOpen((v) => !v);
      }
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, []);

  const pages: Target[] = [
    { to: "/", title: t("nav.home") },
    { to: "/chat/browse", title: t("chat.browse") },
    ...(taskOn ? [{ to: "/tasks", title: t("nav.tasks") }] : []),
    ...(approvalOn ? [{ to: "/approvals", title: t("nav.approvals") }] : []),
    ...(helpdeskOn ? [{ to: "/helpdesk", title: t("nav.helpdesk") }] : []),
    { to: "/members", title: t("nav.members") },
    { to: "/invites", title: t("nav.invites") },
    ...(canManage(current?.role) ? [{ to: "/audit", title: t("nav.audit") }] : []),
    { to: "/profile", title: t("nav.profile") },
  ];

  function go(to: string) {
    setOpen(false);
    void navigate(to);
  }

  const label = t("ui.search", { name: current?.name ?? t("title") });

  return (
    <>
      <button
        type="button"
        aria-label={label}
        onClick={() => setOpen(true)}
        className={cn(
          "flex h-8 w-full min-w-0 items-center gap-2 rounded-md border border-sidebar-border bg-sidebar-foreground/12 px-3 text-sm text-muted-foreground outline-none transition-colors duration-150",
          "hover:bg-sidebar-foreground/20 active:bg-sidebar-foreground/25 focus-visible:ring-2 focus-visible:ring-sidebar-ring motion-safe:active:scale-[0.99]",
          className,
        )}
      >
        <Search aria-hidden="true" className="size-4 shrink-0" />
        <span className="min-w-0 flex-1 truncate text-left">{label}</span>
        <kbd aria-hidden="true" className="hidden rounded-sm border border-sidebar-border px-1.5 font-sans text-xs md:block">
          Ctrl K
        </kbd>
      </button>
      <CommandDialog open={open} onOpenChange={setOpen} title={t("ui.jump")} description={t("ui.jumpHint")} showCloseButton={false}>
        <CommandInput placeholder={t("ui.jumpHint")} />
        <CommandList>
          <CommandEmpty>{t("ui.jumpEmpty")}</CommandEmpty>
          <ChannelGroups onPick={go} />
          <CommandGroup heading={t("ui.pages")}>
            {pages.map((p) => (
              <CommandItem key={p.to} value={p.title} onSelect={() => go(p.to)}>
                {p.title}
              </CommandItem>
            ))}
          </CommandGroup>
        </CommandList>
      </CommandDialog>
    </>
  );
}
