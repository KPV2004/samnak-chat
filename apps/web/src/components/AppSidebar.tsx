import type { ReactNode } from "react";
import { useLocation } from "react-router";
import { ApprovalSidebar } from "../approval/ApprovalSidebar";
import { ConversationList } from "../chat/ConversationList";
import { HelpdeskSidebar } from "../helpdesk/HelpdeskSidebar";
import { useT } from "../i18n/useT";
import { TaskSidebar } from "../task/TaskSidebar";
import { AccountMenu } from "./AccountMenu";
import { AccountSidebar } from "./AccountSidebar";
import { appLabels, appOf } from "./apps";
import { SidebarNav } from "./SidebarNav";
import { WorkspaceSidebar } from "./WorkspaceSidebar";
import { WorkspaceSwitcher } from "./WorkspaceSwitcher";

/**
 * The single left column: the workspace switcher, the app list, and under them the current app's
 * own content (the conversation list, task views, and so on). At `md` it is a 64px icon column; from
 * `lg` the names and the app content show too. Below `md` it is not rendered; the bottom tab bar
 * replaces it.
 */
export function AppSidebar({ onNavigate, noWorkspace = false }: { onNavigate?: () => void; noWorkspace?: boolean }) {
  const t = useT();
  const app = appOf(useLocation().pathname);

  let body: ReactNode = null;
  // Without a workspace the app pages are replaced by one empty state, so the app's own list must
  // not sit there on a loading skeleton that can never finish (BT-7).
  if (noWorkspace) {
    body = null;
  } else if (app === "messenger") {
    body = (
      <div className="flex min-h-0 flex-1 flex-col pt-2">
        <ConversationList />
      </div>
    );
  } else if (app !== null) {
    body = (
      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
        {app === "tasks" && <TaskSidebar />}
        {app === "approvals" && <ApprovalSidebar />}
        {app === "helpdesk" && <HelpdeskSidebar />}
        {app === "workspace" && <WorkspaceSidebar />}
        {app === "account" && <AccountSidebar />}
      </div>
    );
  }

  return (
    <div
      data-sidebar
      className="flex h-full min-h-0 w-full flex-col bg-sidebar"
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a")) onNavigate?.();
      }}
    >
      <div className="shrink-0 px-1.5 pt-2">
        <WorkspaceSwitcher onNavigate={onNavigate} />
      </div>
      <div className="shrink-0 px-1.5 py-2">
        <SidebarNav onNavigate={onNavigate} />
      </div>
      {app !== "messenger" && (
        <div className="hidden h-9 shrink-0 items-baseline border-b border-sidebar-border px-3 lg:flex">
          <p className="m-0 min-w-0 truncate text-sm font-semibold">{t(app ? appLabels[app] : "title")}</p>
        </div>
      )}
      <div className="hidden min-h-0 flex-1 flex-col lg:flex">{body}</div>
      <div className="mt-auto flex shrink-0 items-center border-t border-sidebar-border p-1.5 lg:px-2">
        <AccountMenu current={appOf(useLocation().pathname) === "account"} onNavigate={onNavigate} />
      </div>
    </div>
  );
}
