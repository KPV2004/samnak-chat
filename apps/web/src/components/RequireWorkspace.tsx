import type { Workspace } from "@samnak/core";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { useT } from "../i18n/useT";
import { useWorkspaces } from "../state/queries";

/** Renders its children once a workspace is selected; otherwise explains what to do. */
export function RequireWorkspace({ children }: { children: (workspace: Workspace) => ReactNode }) {
  const t = useT();
  const { query, current } = useWorkspaces();
  if (query.isPending) return <p role="status">{t("loading")}</p>;
  if (query.isError) return <p role="alert">{t("error.generic")}</p>;
  // The shell picks a workspace right after the list loads.
  if (!current && query.data.length > 0) return <p role="status">{t("loading")}</p>;
  if (!current) {
    return (
      <p>
        {t("workspace.none")} <Link to="/workspaces/new">{t("workspace.new")}</Link>
      </p>
    );
  }
  return <>{children(current)}</>;
}
