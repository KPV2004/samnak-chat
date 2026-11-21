import { Inbox, LifeBuoy, Plus, Settings } from "lucide-react";
import { useLocation } from "react-router";
import { SidebarAction, SidebarLink } from "../components/SidebarLink";
import { SidebarSection } from "../components/SidebarSection";
import { useT } from "../i18n/useT";
import { useModuleEnabled } from "../modules";
import { helpdeskPath, helpdeskViewOf, useHelpdeskQueues, useHelpdeskRole } from "./view";

/**
 * The sidebar of the Helpdesk app: "New ticket", My requests, the queues (for agents and admins:
 * all of them, then one entry per queue the user is an agent of) and Settings (admins).
 */
export function HelpdeskSidebar() {
  const t = useT();
  const location = useLocation();
  const queues = useHelpdeskQueues(useModuleEnabled("helpdesk"));
  const { admin, agent, agentOf } = useHelpdeskRole(queues.data?.items ?? []);
  const params = new URLSearchParams(location.search);
  const onList = location.pathname === "/helpdesk";
  const view = helpdeskViewOf(params.get("view"));
  const queueId = params.get("queue") ?? "";
  return (
    <div className="grid gap-2">
      <SidebarAction to="/helpdesk?new=1" icon={Plus}>
        {t("helpdesk.new")}
      </SidebarAction>
      <nav aria-label={t("helpdesk.views")} className="grid gap-0.5">
        <SidebarLink to={helpdeskPath("mine")} icon={LifeBuoy} current={onList && view === "mine"}>
          {t("helpdesk.tabs.mine")}
        </SidebarLink>
        {(agent || admin) && (
          <SidebarSection id="helpdesk-queues" title={t("helpdesk.tabs.queue")}>
            <SidebarLink to={helpdeskPath("queue")} icon={Inbox} current={onList && view === "queue" && queueId === ""}>
              {t("helpdesk.allQueues")}
            </SidebarLink>
            {agentOf.map((q) => (
              <SidebarLink key={q.id} to={helpdeskPath("queue", q.id)} icon={Inbox} current={onList && view === "queue" && queueId === q.id}>
                {q.name}
              </SidebarLink>
            ))}
          </SidebarSection>
        )}
        {admin && (
          <SidebarLink to="/helpdesk/settings" icon={Settings} current={location.pathname === "/helpdesk/settings"}>
            {t("helpdesk.settings")}
          </SidebarLink>
        )}
      </nav>
    </div>
  );
}
