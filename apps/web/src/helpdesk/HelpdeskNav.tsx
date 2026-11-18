import { LifeBuoy } from "lucide-react";
import { NavItem } from "../components/NavItem";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";

/** The "Helpdesk" link in the shell navigation. It is not there while `/meta` lacks helpdesk. */
export function HelpdeskNav() {
  const t = useT();
  const enabled = useModuleEnabled("helpdesk");
  if (!enabled) return null;
  return (
    <NavItem to="/helpdesk" icon={LifeBuoy}>
      {t("nav.helpdesk")}
    </NavItem>
  );
}
