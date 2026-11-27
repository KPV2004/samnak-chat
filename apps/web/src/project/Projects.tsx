import { FolderKanban } from "lucide-react";
import { EmptyState } from "../components/EmptyState";
import { Page } from "../components/Page";
import { useT } from "../i18n/useT";

/**
 * Projects are designed (design.md §Screens) but the server has no project entity or endpoint yet.
 * The route exists only behind the `project` module flag; with no API there is nothing honest to
 * list, so it says so instead of inventing data.
 */
export function Projects() {
  const t = useT();
  return (
    <Page title={t("rail.projects")} width="wide">
      <EmptyState icon={FolderKanban} title={t("project.backendPending")} />
    </Page>
  );
}
