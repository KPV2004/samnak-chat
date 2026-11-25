import { Building2 } from "lucide-react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { EmptyState } from "./EmptyState";
import { useT } from "../i18n/useT";

/**
 * What a signed-in user without a workspace sees instead of the app (BT-7). Nothing that needs a
 * workspace is requested or shown, so the chat does not sit on "loading" forever.
 */
export function NoWorkspace() {
  const t = useT();
  return (
    <div data-page className="mx-auto w-full max-w-2xl p-4 md:p-6">
      <EmptyState
        icon={Building2}
        title={t("workspace.none")}
        description={t("workspace.noneBody")}
        action={
          <Button asChild>
            <Link to="/workspaces/new">{t("workspace.new")}</Link>
          </Button>
        }
      />
    </div>
  );
}
