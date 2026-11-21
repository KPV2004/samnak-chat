import { Inbox, Plus, Send } from "lucide-react";
import { useLocation } from "react-router";
import { CountBadge, SidebarAction, SidebarLink } from "../components/SidebarLink";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { approvalPath, approvalViewOf, useWaitingCount } from "./requests";

/**
 * The sidebar of the Approvals app: a "New request" button, then the two views of the page as
 * links (`?view=`). "Waiting for me" carries the count of the list it opens.
 */
export function ApprovalSidebar() {
  const t = useT();
  const location = useLocation();
  const onPage = location.pathname === "/approvals";
  const view = approvalViewOf(new URLSearchParams(location.search).get("view"));
  const waiting = useWaitingCount(useModuleEnabled("approval"));
  return (
    <div className="grid gap-2">
      <SidebarAction to="/approvals?new=1" icon={Plus}>
        {t("approval.new")}
      </SidebarAction>
      <nav aria-label={t("approval.views")} className="grid gap-0.5">
        <SidebarLink
          to={approvalPath("waiting")}
          icon={Inbox}
          current={onPage && view === "waiting"}
          trailing={<CountBadge n={waiting.n} capped={waiting.more} label={t("approval.waitingCount", { n: waiting.n })} />}
        >
          {t("approval.toApprove")}
        </SidebarLink>
        <SidebarLink to={approvalPath("mine")} icon={Send} current={onPage && view === "mine"}>
          {t("approval.mine")}
        </SidebarLink>
      </nav>
    </div>
  );
}
