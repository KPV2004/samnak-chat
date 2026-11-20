import { BellRing, House, Laptop, LogOut, MessagesSquare, ScrollText, UserPlus, UserRound, Users } from "lucide-react";
import { useMatch } from "react-router";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { ApprovalNav } from "../approval/ApprovalNav";
import { ConversationList } from "../chat/ConversationList";
import { Sidebar as ChannelList } from "../chat/Sidebar";
import { HelpdeskNav } from "../helpdesk/HelpdeskNav";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSession } from "../state/session";
import { TaskNavLink } from "../task/TaskNavLink";
import { LanguageSelect } from "./LanguageSelect";
import { NavItem } from "./NavItem";
import { SidebarSection } from "./SidebarSection";
import { UserAvatar } from "./UserAvatar";

/**
 * The dark sidebar: the workspace name, the main links, the channel and direct-message sections,
 * the apps, the workspace pages and the signed-in user. In Messenger (`/chat`) the links and
 * sections give way to the conversation list, and the workspace rail switches app. The shell shows
 * it beside the page on md and wider, and inside a Sheet (after the workspace rail) below.
 * `onNavigate` runs after a link inside it was followed, so the Sheet can close.
 */
export function AppSidebar({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const session = useSession();
  const me = useMe();
  const { current } = useWorkspaces();
  const notifyEnabled = useModuleEnabled("notify");
  const taskEnabled = useModuleEnabled("task");
  const approvalEnabled = useModuleEnabled("approval");
  const helpdeskEnabled = useModuleEnabled("helpdesk");
  const inMessenger = useMatch("/chat/*") !== null;

  return (
    <div
      className="on-rail flex h-full min-h-0 w-full flex-col bg-sidebar"
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a")) onNavigate?.();
      }}
    >
      <div className="flex h-11 shrink-0 items-center border-b border-sidebar-border px-4">
        <p className="m-0 truncate text-base font-semibold">{current?.name ?? t("title")}</p>
      </div>
      {inMessenger ? (
        <div className="flex min-h-0 flex-1 flex-col pt-2">
          <ConversationList />
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
          <nav aria-label={t("nav.label")} className="grid gap-2">
            <div className="grid gap-0.5">
              <NavItem to="/" icon={House} end>
                {t("nav.home")}
              </NavItem>
              <NavItem to="/chat" icon={MessagesSquare}>
                {t("nav.chat")}
              </NavItem>
            </div>
            <ChannelList />
            {(taskEnabled || approvalEnabled || helpdeskEnabled) && (
              <SidebarSection id="apps" title={t("nav.apps")}>
                <TaskNavLink />
                <ApprovalNav />
                <HelpdeskNav />
              </SidebarSection>
            )}
            <SidebarSection id="workspace" title={t("nav.workspace")}>
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
            </SidebarSection>
            <SidebarSection id="account" title={t("ui.account")}>
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
            </SidebarSection>
          </nav>
        </div>
      )}
      <div className="grid shrink-0 gap-2 border-t border-sidebar-border p-2">
        <div className="md:hidden">
          <LanguageSelect />
        </div>
        <div className="flex items-center gap-2">
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
    </div>
  );
}
