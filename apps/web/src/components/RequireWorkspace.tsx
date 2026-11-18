import type { Workspace } from "@samnak/core";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useT } from "../i18n/useT";
import { useWorkspaces } from "../state/queries";
import { AccountLoadError, AccountLoading } from "./AccountStates";
import { EmptyState } from "./EmptyState";

/** Renders its children once a workspace is selected; otherwise explains what to do. */
export function RequireWorkspace({ children }: { children: (workspace: Workspace) => ReactNode }) {
  const t = useT();
  const { query, current } = useWorkspaces();
  if (query.isPending) return <WorkspaceLoading />;
  if (query.isError) return <AccountLoadError message={t("error.generic")} />;
  // The shell picks a workspace right after the list loads.
  if (!current && query.data.length > 0) return <WorkspaceLoading />;
  if (!current) {
    return (
      <EmptyState
        title={t("workspace.none")}
        action={
          <Button asChild>
            <Link to="/workspaces/new">{t("workspace.new")}</Link>
          </Button>
        }
      />
    );
  }
  return <>{children(current)}</>;
}

function WorkspaceLoading() {
  return (
    <AccountLoading>
      <Skeleton className="h-16 w-full" />
      <Skeleton className="h-16 w-full" />
    </AccountLoading>
  );
}
