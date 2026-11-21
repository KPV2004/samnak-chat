import type { ReactNode } from "react";
import { useLocation } from "react-router";
import { ApprovalSidebar } from "../approval/ApprovalSidebar";
import { ConversationList } from "../chat/ConversationList";
import { HelpdeskSidebar } from "../helpdesk/HelpdeskSidebar";
import { useT } from "../i18n/useT";
import { useWorkspaces } from "../state/queries";
import { TaskSidebar } from "../task/TaskSidebar";
import { AccountSidebar } from "./AccountSidebar";
import { appLabels, appOf } from "./apps";
import { WorkspaceSidebar } from "./WorkspaceSidebar";

/**
 * The dark sidebar beside the page: only the app the workspace rail has picked. Messenger shows the
 * conversation list; Tasks, Approvals and Helpdesk show their views; Workspace shows its pages; the
 * account pages show a small Account sidebar. The header names the app, with the workspace beside
 * it (Messenger's list heading does that itself). The shell shows it on md and wider, and inside a Sheet (after the rail) below; `onNavigate`
 * runs after a link inside it was followed, so the Sheet can close.
 */
export function AppSidebar({ onNavigate }: { onNavigate?: () => void }) {
  const t = useT();
  const { current } = useWorkspaces();
  const app = appOf(useLocation().pathname);

  let body: ReactNode = null;
  if (app === "messenger") {
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
      className="on-rail flex h-full min-h-0 w-full flex-col bg-sidebar"
      onClick={(e) => {
        if ((e.target as HTMLElement).closest("a")) onNavigate?.();
      }}
    >
      {/* The Messenger list has its own heading, which names the app. */}
      {app !== "messenger" && (
        <div className="flex h-11 shrink-0 items-baseline gap-2 border-b border-sidebar-border px-4 pt-2.5">
          <p className="m-0 shrink-0 text-base font-semibold">{t(app ? appLabels[app] : "title")}</p>
          {current && <p className="m-0 min-w-0 truncate text-xs text-muted-foreground">{current.name}</p>}
        </div>
      )}
      {body}
    </div>
  );
}
