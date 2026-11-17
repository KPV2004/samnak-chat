import { NavLink } from "react-router";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "./useModule";

/** The header links of the approval module: its requests and the groups they route to. Hidden while /meta lacks approval. */
export function ApprovalNav() {
  const t = useT();
  const { enabled } = useModuleEnabled("approval");
  if (!enabled) return null;
  return (
    <>
      <NavLink to="/approvals">{t("nav.approvals")}</NavLink>
      <NavLink to="/groups">{t("nav.groups")}</NavLink>
    </>
  );
}
