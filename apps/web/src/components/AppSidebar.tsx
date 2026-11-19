import { BellRing, House, Laptop, LogOut, MessagesSquare, Plus, ScrollText, UserPlus, UserRound, Users } from "lucide-react";
import { Link, useLocation } from "react-router";
import { Button } from "@/components/ui/button";
import { NativeSelect } from "@/components/ui/native-select";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ApprovalNav } from "../approval/ApprovalNav";
import { Sidebar as ChannelList } from "../chat/Sidebar";
import { HelpdeskNav } from "../helpdesk/HelpdeskNav";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSession } from "../state/session";
import { TaskNavLink } from "../task/TaskNavLink";
import { Brand } from "./Brand";
import { NavItem } from "./NavItem";
import { UserAvatar } from "./UserAvatar";

/**
 * The left column of the app: product name, workspace switch, the main links, the channel list (in
 * chat) and the signed-in user. The shell shows it beside the page on md and wider and inside a Sheet below.
 * `onNavigate` runs after a link inside it was followed, so the Sheet can close.
 */
export function AppSidebar({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const me = useMe();
  const { workspaces, current } = useWorkspaces();
  const notifyEnabled = useModuleEnabled("notify");
  // The channel list looks up member names, so it only mounts where chat is open, as before.
  const inChat = useLocation().pathname.startsWith("/chat");

  return (
    <div
      className="flex h-full min-h-0 flex-col bg-sidebar text-sidebar-foreground"
      onClick={(e) => {
        const link = (e.target as HTMLElement).closest("a");
        // "Chat" itself leaves a Sheet open, because the channel list it reveals is what the visitor came for.
        if (link && link.getAttribute("href") !== "/chat") onNavigate?.();
      }}
    >
      <div className="flex h-12 shrink-0 items-center px-4">
        <Brand />
      </div>
      <div className="grid shrink-0 gap-1 px-2 pb-2">
        {workspaces && workspaces.length > 0 && (
          <label className="block">
            <span className="sr-only">{t("workspace.switch")}</span>
            <NativeSelect
              containerClassName="w-full"
              className="font-medium"
              value={current?.id ?? ""}
              onChange={(e) => session.setWorkspaceId(e.target.value)}
            >
              {workspaces.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </NativeSelect>
          </label>
        )}
        <Button asChild variant="ghost" size="sm" className="justify-start px-2 text-muted-foreground">
          <Link to="/workspaces/new">
            <Plus />
            {t("workspace.new")}
          </Link>
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-2 pb-3">
        <nav aria-label={t("nav.label")} className="mb-4 grid gap-0.5">
          <NavItem to="/" icon={House} end>
            {t("nav.home")}
          </NavItem>
          <NavItem to="/chat" icon={MessagesSquare}>
            {t("nav.chat")}
          </NavItem>
          <TaskNavLink />
          <ApprovalNav />
          <HelpdeskNav />
          <NavItem to="/members" icon={Users}>
            {t("nav.members")}
          </NavItem>
          <NavItem to="/invites" icon={UserPlus}>
            {t("nav.invites")}
          </NavItem>
          {canManage(current?.role) && (
            <NavItem to="/audit" icon={ScrollText}>
              {t("nav.audit")}
            </NavItem>
          )}
          <NavItem to="/profile" icon={UserRound}>
            {t("nav.profile")}
          </NavItem>
          <NavItem to="/devices" icon={Laptop}>
            {t("nav.devices")}
          </NavItem>
          {notifyEnabled && (
            <NavItem to="/notifications/settings" icon={BellRing}>
              {t("nav.notifySettings")}
            </NavItem>
          )}
        </nav>
        {inChat && <ChannelList />}
      </div>
      <div className="flex shrink-0 items-center gap-2 border-t border-sidebar-border p-2">
        {me.data && (
          <>
            <UserAvatar name={me.data.display_name} />
            <div className="min-w-0 flex-1 leading-snug">
              <p className="m-0 truncate text-sm font-medium">{me.data.display_name}</p>
              <p className="m-0 truncate text-xs text-muted-foreground">{me.data.email}</p>
            </div>
          </>
        )}
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              className="ml-auto shrink-0"
              aria-label={t("signOut")}
              onClick={() => void session.logout(false)}
            >
              <LogOut />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t("signOut")}</TooltipContent>
        </Tooltip>
      </div>
    </div>
  );
}
