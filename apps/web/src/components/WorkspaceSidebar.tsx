import { ScrollText, UserPlus, Users, UsersRound } from "lucide-react";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { canManage, useWorkspaces } from "../state/queries";
import { NavItem } from "./NavItem";

/** The sidebar of the Workspace app: members and invites for everyone, groups while approval is on, the audit log for owners and admins. */
export function WorkspaceSidebar() {
  const t = useT();
  const { current } = useWorkspaces();
  const approval = useModuleEnabled("approval");
  return (
    <nav aria-label={t("rail.workspace")} className="grid gap-0.5">
      <NavItem to="/members" icon={Users}>
        {t("nav.members")}
      </NavItem>
      <NavItem to="/invites" icon={UserPlus}>
        {t("nav.invites")}
      </NavItem>
      {approval && (
        <NavItem to="/groups" icon={UsersRound}>
          {t("nav.groups")}
        </NavItem>
      )}
      {canManage(current?.role) && (
        <NavItem to="/audit" icon={ScrollText}>
          {t("nav.audit")}
        </NavItem>
      )}
    </nav>
  );
}
