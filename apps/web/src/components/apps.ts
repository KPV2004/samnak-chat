import type { MessageKey } from "@/i18n/messages";

/** The apps of the workspace rail. Each owns the sidebar beside it. */
export type AppId = "messenger" | "tasks" | "approvals" | "helpdesk" | "workspace" | "account";

const onPath = (pathname: string, base: string) => pathname === base || pathname.startsWith(`${base}/`);

const routes: { app: AppId; bases: string[] }[] = [
  { app: "messenger", bases: ["/chat"] },
  { app: "tasks", bases: ["/tasks"] },
  { app: "approvals", bases: ["/approvals"] },
  { app: "helpdesk", bases: ["/helpdesk"] },
  { app: "workspace", bases: ["/members", "/invites", "/groups", "/audit", "/workspaces"] },
  { app: "account", bases: ["/profile", "/devices", "/notifications"] },
];

/** The app a path belongs to; null for a page outside every app (accepting an invite). */
export function appOf(pathname: string): AppId | null {
  return routes.find((r) => r.bases.some((b) => onPath(pathname, b)))?.app ?? null;
}

/** The name an app shows on the rail and as its sidebar heading. */
export const appLabels: Record<AppId, MessageKey> = {
  messenger: "rail.messenger",
  tasks: "rail.tasks",
  approvals: "rail.approvals",
  helpdesk: "rail.helpdesk",
  workspace: "rail.workspace",
  account: "ui.account",
};
