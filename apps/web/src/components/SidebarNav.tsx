import { ClipboardCheck, House, LifeBuoy, ListChecks, MessagesSquare, Building2, type LucideIcon } from "lucide-react";
import { Link, useLocation } from "react-router";
import { Badge } from "@/components/ui/badge";
import type { MessageKey } from "@/i18n/messages";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useWaitingCount } from "../approval/requests";
import { useModuleEnabled } from "../modules";
import { usePreferences } from "../notify/usePreferences";
import { useChatState } from "../state/chat";
import { appOf, type AppId } from "./apps";

/** Unread chats for the badge: muted channels are left out, as they send no notification. */
export function useMessengerUnread(): { unread: number; mentions: number } {
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

interface App {
  id: AppId;
  to: string;
  icon: LucideIcon;
  label: MessageKey;
  show: boolean;
}

/**
 * The apps of the single left column. At `md` it is the 64px icon column; from `lg` it shows the
 * names too. The current one is filled and marked `aria-current`; an app whose module is off is
 * not there at all.
 */
export function SidebarNav({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const { pathname } = useLocation();
  const task = useModuleEnabled("task");
  const approval = useModuleEnabled("approval");
  const helpdesk = useModuleEnabled("helpdesk");
  const { unread, mentions } = useMessengerUnread();
  // Only reads what the Approvals list already loaded; the nav makes no request of its own.
  const waiting = useWaitingCount(false);
  const apps: App[] = [
    { id: "home", to: "/", icon: House, label: "nav.home", show: true },
    { id: "messenger", to: "/chat", icon: MessagesSquare, label: "rail.messenger", show: true },
    { id: "tasks", to: "/tasks", icon: ListChecks, label: "rail.tasks", show: task },
    { id: "approvals", to: "/approvals", icon: ClipboardCheck, label: "rail.approvals", show: approval },
    { id: "helpdesk", to: "/helpdesk", icon: LifeBuoy, label: "rail.helpdesk", show: helpdesk },
    { id: "workspace", to: "/members", icon: Building2, label: "rail.workspace", show: true },
  ];
  return (
    <nav aria-label={t("rail.apps")} className="grid gap-0.5">
      {apps
        .filter((a) => a.show)
        .map((a) => {
          const current = appOf(pathname) === a.id;
          return (
            <Link
              key={a.to}
              to={a.to}
              aria-current={current ? "page" : undefined}
              onClick={onNavigate}
              className={cn(
                "group/app flex h-9 items-center gap-2.5 rounded-md px-2 text-sm text-muted-foreground outline-none transition-colors duration-150",
                "hover:bg-sidebar-accent hover:text-sidebar-accent-foreground active:bg-sidebar-accent/60",
                "focus-visible:ring-2 focus-visible:ring-sidebar-ring",
                "aria-[current=page]:bg-sidebar-primary aria-[current=page]:font-semibold aria-[current=page]:text-sidebar-primary-foreground",
              )}
            >
              <span className="relative grid size-6 shrink-0 place-items-center">
                <a.icon aria-hidden="true" className="size-5" />
                {a.id === "approvals" && waiting.n > 0 && (
                  <Badge className="absolute -top-1.5 -right-1.5 h-4 min-w-4 px-1 text-[12px] ring-2 ring-sidebar">
                    <span aria-hidden="true">{waiting.more ? `${waiting.n}+` : waiting.n}</span>
                    <span className="sr-only">{t("approval.waitingCount", { n: waiting.n })}</span>
                  </Badge>
                )}
                {a.id === "messenger" && unread > 0 && (
                  <Badge
                    variant={mentions > 0 ? "destructive" : "default"}
                    className="absolute -top-1.5 -right-1.5 h-4 min-w-4 px-1 text-[12px] ring-2 ring-sidebar"
                  >
                    <span aria-hidden="true">{unread > 99 ? "99+" : unread}</span>
                    <span className="sr-only">{t("chat.unread", { n: unread })}</span>
                  </Badge>
                )}
              </span>
              <span className="hidden min-w-0 truncate lg:inline">{t(a.label)}</span>
            </Link>
          );
        })}
    </nav>
  );
}
