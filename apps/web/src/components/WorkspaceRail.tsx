import type { Workspace } from "@samnak/core";
import { ClipboardCheck, LifeBuoy, ListChecks, MessagesSquare, Plus, Users, type LucideIcon } from "lucide-react";
import { Link, useLocation } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { MessageKey } from "@/i18n/messages";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useModuleEnabled } from "../modules";
import { usePreferences } from "../notify/usePreferences";
import { useChatState } from "../state/chat";
import { useWorkspaces } from "../state/queries";
import { useSession } from "../state/session";
import { initialOf } from "./UserAvatar";

/** The two characters a workspace tile shows: the first letters of its first two words, else its first two letters. */
export function workspaceInitials(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  if (words.length >= 2) return initialOf(words[0]!) + initialOf(words[1]!);
  const letters = Array.from(name.trim()).slice(0, 2).join("");
  return letters.toLocaleUpperCase() || "?";
}

const tileClass = cn(
  "relative grid size-9 place-items-center rounded-lg text-sm font-semibold outline-none transition-[background-color,scale] duration-150",
  "focus-visible:ring-2 focus-visible:ring-sidebar-ring focus-visible:ring-offset-2 focus-visible:ring-offset-rail",
  "motion-safe:hover:-translate-y-px motion-safe:active:scale-95",
);

interface App {
  to: string;
  /** Paths that mark the app current, besides `to`. */
  also?: string[];
  icon: LucideIcon;
  label: MessageKey;
  show: boolean;
}

const onPath = (pathname: string, base: string) => pathname === base || pathname.startsWith(`${base}/`);

/** Unread chats for the badge on Messenger: muted ones are left out, as they send no notification. */
function useMessengerUnread(): { unread: number; mentions: number } {
  const { channels } = useChatState();
  const prefs = usePreferences();
  const muted = new Set(prefs.data?.muted_channel_ids ?? []);
  let unread = 0;
  let mentions = 0;
  for (const s of Object.values(channels)) {
    if (muted.has(s.channel.id)) continue;
    unread += s.unreadCount;
    mentions += s.mentionCount;
  }
  return { unread, mentions };
}

/**
 * The apps under the workspace tiles, each an icon with its name beneath. The current one is filled
 * mint and marked `aria-current`; an app whose module is off is not there.
 */
function AppLinks({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const { pathname } = useLocation();
  const task = useModuleEnabled("task");
  const approval = useModuleEnabled("approval");
  const helpdesk = useModuleEnabled("helpdesk");
  const { unread, mentions } = useMessengerUnread();
  const apps: App[] = [
    { to: "/chat", icon: MessagesSquare, label: "rail.messenger", show: true },
    { to: "/tasks", icon: ListChecks, label: "rail.tasks", show: task },
    { to: "/approvals", also: ["/groups"], icon: ClipboardCheck, label: "rail.approvals", show: approval },
    { to: "/helpdesk", icon: LifeBuoy, label: "rail.helpdesk", show: helpdesk },
    { to: "/members", icon: Users, label: "rail.members", show: true },
  ];
  return (
    <nav aria-label={t("rail.apps")} className="flex w-full flex-col items-center gap-1 px-0.5">
      {apps
        .filter((a) => a.show)
        .map((a) => {
          const current = [a.to, ...(a.also ?? [])].some((base) => onPath(pathname, base));
          return (
            <Link
              key={a.to}
              to={a.to}
              aria-current={current ? "page" : undefined}
              onClick={onNavigate}
              className="group/app flex w-full flex-col items-center gap-0.5 rounded-lg py-1 text-[11px] leading-tight text-muted-foreground outline-none transition-colors duration-150 hover:text-sidebar-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring aria-[current=page]:font-medium aria-[current=page]:text-sidebar-foreground"
            >
              <span
                className={cn(
                  "relative grid size-9 place-items-center rounded-lg transition-[background-color,scale] duration-150",
                  "group-hover/app:bg-sidebar-accent group-active/app:bg-sidebar-accent/60 motion-safe:group-hover/app:-translate-y-px motion-safe:group-active/app:scale-95",
                  "group-aria-[current=page]/app:bg-sidebar-primary group-aria-[current=page]/app:text-sidebar-primary-foreground",
                )}
              >
                <a.icon aria-hidden="true" className="size-5" />
                {a.to === "/chat" && unread > 0 && (
                  <Badge variant={mentions > 0 ? "destructive" : "default"} className="absolute -top-1 -right-1.5 h-4 min-w-4 px-1 text-[10px] ring-2 ring-rail">
                    <span aria-hidden="true">{unread > 99 ? "99+" : unread}</span>
                    <span className="sr-only">{t("chat.unread", { n: unread })}</span>
                  </Badge>
                )}
              </span>
              <span className="max-w-full truncate">{t(a.label)}</span>
            </Link>
          );
        })}
    </nav>
  );
}

/**
 * The narrow dark column on the far left: one tile per workspace (the current one is filled), a
 * tile to make a new one, and under them the apps. Switching only changes the selected workspace,
 * as the old select did.
 */
export function WorkspaceRail({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const { workspaces, current } = useWorkspaces();

  return (
    <div className="on-rail flex w-16 shrink-0 flex-col items-center gap-2 overflow-y-auto bg-rail py-2">
      <nav aria-label={t("workspace.list")} className="flex flex-col items-center gap-2">
        {workspaces?.map((w: Workspace) => {
          const isCurrent = w.id === current?.id;
          return (
            <Tooltip key={w.id}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  aria-label={w.name}
                  aria-current={isCurrent ? "true" : undefined}
                  className={cn(
                    tileClass,
                    isCurrent
                      ? "bg-sidebar-primary text-sidebar-primary-foreground"
                      : "bg-sidebar-foreground/15 text-sidebar-foreground hover:bg-sidebar-foreground/25",
                  )}
                  onClick={() => {
                    session.setWorkspaceId(w.id);
                    onNavigate?.();
                  }}
                >
                  {isCurrent && (
                    <span aria-hidden="true" className="absolute top-1/2 -left-2.5 h-5 w-1 -translate-y-1/2 rounded-r-full bg-sidebar-primary" />
                  )}
                  <span aria-hidden="true">{workspaceInitials(w.name)}</span>
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">{w.name}</TooltipContent>
            </Tooltip>
          );
        })}
        <Tooltip>
          <TooltipTrigger asChild>
            <Link
              to="/workspaces/new"
              aria-label={t("workspace.new")}
              className={cn(tileClass, "border border-dashed border-sidebar-foreground/40 text-sidebar-foreground hover:bg-sidebar-accent")}
              onClick={onNavigate}
            >
              <Plus aria-hidden="true" className="size-4" />
            </Link>
          </TooltipTrigger>
          <TooltipContent side="right">{t("workspace.new")}</TooltipContent>
        </Tooltip>
      </nav>
      <Separator className="w-8 bg-sidebar-border" />
      <AppLinks onNavigate={onNavigate} />
    </div>
  );
}
