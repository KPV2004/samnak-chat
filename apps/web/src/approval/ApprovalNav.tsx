import { ClipboardCheck, UsersRound } from "lucide-react";
import { NavItem } from "../components/NavItem";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";

/** The sidebar links of the approval module: its requests and the groups they route to. Hidden while /meta lacks approval. */
export function ApprovalNav() {
  const t = useT();
  const enabled = useModuleEnabled("approval");
  if (!enabled) return null;
  return (
    <>
      <NavItem to="/approvals" icon={ClipboardCheck}>
        {t("nav.approvals")}
      </NavItem>
      <NavItem to="/groups" icon={UsersRound}>
        {t("nav.groups")}
      </NavItem>
    </>
  );
}
