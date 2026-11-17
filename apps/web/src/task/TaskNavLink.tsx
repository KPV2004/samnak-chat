import { ListChecks } from "lucide-react";
import { NavItem } from "../components/NavItem";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";

/** The "My tasks" link in the shell navigation (TASK-04). It is not there when `/meta` lacks task. */
export function TaskNavLink() {
  const t = useT();
  const enabled = useModuleEnabled("task");
  if (!enabled) return null;
  return (
    <NavItem to="/tasks" icon={ListChecks}>
      {t("nav.tasks")}
    </NavItem>
  );
}
