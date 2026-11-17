import { NavLink } from "react-router";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";

/** The "My tasks" link in the shell navigation (TASK-04). It is not there when `/meta` lacks task. */
export function TaskNavLink() {
  const t = useT();
  const enabled = useModuleEnabled("task");
  if (!enabled) return null;
  return <NavLink to="/tasks">{t("nav.tasks")}</NavLink>;
}
