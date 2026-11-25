import { ClipboardCheck, Ellipsis, FolderKanban, House, LifeBuoy, ListChecks, MessagesSquare, Settings } from "lucide-react";
import { useState } from "react";
import { NavLink, useLocation } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useT } from "@/i18n/useT";
import { cn } from "@/lib/utils";
import { useWaitingCount } from "../approval/requests";
import { useModuleEnabled } from "../modules";
import { AccountMenu } from "./AccountMenu";
import { appOf } from "./apps";
import { useMessengerUnread } from "./SidebarNav";

const itemClass =
  "relative flex min-h-14 flex-col items-center justify-center gap-0.5 px-1 text-[12px] leading-tight outline-none transition-colors duration-150 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring aria-[current=page]:font-semibold aria-[current=page]:text-primary";

/**
 * The phone navigation: Home, Chat, Tasks, Approvals and More, 56px plus the safe-area inset.
 * Inside an open conversation it is hidden, so the composer keeps the bottom edge.
 */
export function TabBar() {
  const t = useT();
  const { pathname } = useLocation();
  const { unread, mentions } = useMessengerUnread();
  const waiting = useWaitingCount(false);
  const task = useModuleEnabled("task");
  const approval = useModuleEnabled("approval");
  const helpdesk = useModuleEnabled("helpdesk");
  const project = useModuleEnabled("project");
  const [more, setMore] = useState(false);

  // A conversation fills the screen; its own header has the back button.
  if (/^\/chat\/[^/]+$/.test(pathname) && !["/chat/browse", "/chat/new", "/chat/dm"].includes(pathname)) return null;

  const items = [
    { to: "/", icon: House, label: t("nav.home"), show: true, badge: 0 },
    { to: "/chat", icon: MessagesSquare, label: t("rail.messenger"), show: true, badge: unread },
    { to: "/tasks", icon: ListChecks, label: t("rail.tasks"), show: task, badge: 0 },
    { to: "/approvals", icon: ClipboardCheck, label: t("rail.approvals"), show: approval, badge: waiting.n },
  ];

  return (
    <nav
      aria-label={t("tab.label")}
      className="sticky bottom-0 z-(--z-sticky) grid shrink-0 grid-cols-5 border-t bg-card pb-[env(safe-area-inset-bottom)] md:hidden"
    >
      {items
        .filter((i) => i.show)
        .map((i) => (
          <NavLink key={i.to} to={i.to} end={i.to === "/"} className={itemClass}>
            <i.icon aria-hidden="true" className="size-6" />
            <span className="max-w-full truncate">{i.label}</span>
            {i.badge > 0 && (
              <Badge
                variant={i.to === "/chat" && mentions > 0 ? "destructive" : "default"}
                className="absolute top-1.5 left-1/2 ml-3 h-4 min-w-4 px-1 text-[12px]"
              >
                {i.badge > 99 ? "99+" : i.badge}
              </Badge>
            )}
          </NavLink>
        ))}
      <button type="button" className={itemClass} aria-expanded={more} onClick={() => setMore(true)}>
        <Ellipsis aria-hidden="true" className="size-6" />
        <span>{t("tab.more")}</span>
      </button>
      <Sheet open={more} onOpenChange={setMore}>
        <SheetContent side="bottom" className="gap-0" aria-describedby={undefined}>
          <SheetHeader>
            <SheetTitle>{t("tab.more")}</SheetTitle>
          </SheetHeader>
          <nav aria-label={t("tab.more")} className="grid gap-0.5 px-3 pb-3">
            {project && (
              <MoreLink to="/projects" icon={FolderKanban} label={t("rail.projects")} onClick={() => setMore(false)} />
            )}
            {helpdesk && <MoreLink to="/helpdesk" icon={LifeBuoy} label={t("rail.helpdesk")} onClick={() => setMore(false)} />}
            <MoreLink to="/members" icon={Settings} label={t("rail.workspace")} onClick={() => setMore(false)} />
          </nav>
          <div className="flex items-center gap-2 border-t px-3 py-2">
            <AccountMenu current={appOf(pathname) === "account"} onNavigate={() => setMore(false)} />
            <span className="text-sm text-muted-foreground">{t("tab.moreHint")}</span>
          </div>
        </SheetContent>
      </Sheet>
    </nav>
  );
}

function MoreLink({
  to,
  icon: Icon,
  label,
  onClick,
}: {
  to: string;
  icon: typeof House;
  label: string;
  onClick: () => void;
}) {
  return (
    <NavLink
      to={to}
      onClick={onClick}
      className={cn(
        "flex h-10 items-center gap-2.5 rounded-md px-2 text-sm outline-none transition-colors duration-150",
        "hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring aria-[current=page]:bg-accent aria-[current=page]:font-semibold",
      )}
    >
      <Icon aria-hidden="true" className="size-5 text-muted-foreground" />
      {label}
    </NavLink>
  );
}
