import { BellRing, Laptop, UserRound } from "lucide-react";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { NavItem } from "./NavItem";

/** The small sidebar of the account pages: profile, devices and, while notify is on, notification settings. */
export function AccountSidebar() {
  const t = useT();
  const notify = useModuleEnabled("notify");
  return (
    <nav aria-label={t("ui.account")} className="grid gap-0.5">
      <NavItem to="/profile" icon={UserRound}>
        {t("nav.profile")}
      </NavItem>
      <NavItem to="/devices" icon={Laptop}>
        {t("nav.devices")}
      </NavItem>
      {notify && (
        <NavItem to="/notifications/settings" icon={BellRing}>
          {t("nav.notifySettings")}
        </NavItem>
      )}
    </nav>
  );
}
