import { useQuery } from "@tanstack/react-query";
import { Plus, Settings } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AccountLoadError } from "../components/AccountStates";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { NewTicketDialog } from "./NewTicketDialog";
import { TicketList } from "./TicketList";

/**
 * The Helpdesk page (HD-01, HD-10): "My requests" for everyone, and the queue
 * list for agents and admins. A ticket is opened from the page's primary
 * button; settings are for owners and admins.
 */
export function Helpdesk() {
  return <RequireWorkspace>{() => <HelpdeskView />}</RequireWorkspace>;
}

function HelpdeskView() {
  const t = useT();
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
  const me = useMe().data?.id;
  const workspace = useWorkspaces().current;
  const [tab, setTab] = useState<"mine" | "queue">("mine");
  const [creating, setCreating] = useState(false);

  const queues = useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: workspaceId !== null,
  });
  const items = queues.data?.items ?? [];
  const admin = canManage(workspace?.role);
  const agent = me !== undefined && items.some((q) => q.agent_ids.includes(me));

  return (
    <Page
      title={t("helpdesk.title")}
      width="wide"
      description={t("helpdesk.settings.body")}
      actions={
        <>
          {admin && (
            <Button asChild variant="outline">
              <Link to="/helpdesk/settings">
                <Settings />
                {t("helpdesk.settings")}
              </Link>
            </Button>
          )}
          <Button type="button" onClick={() => setCreating(true)}>
            <Plus />
            {t("helpdesk.new")}
          </Button>
        </>
      }
    >
      {queues.isError && <AccountLoadError message={t("helpdesk.loadError")} onRetry={() => void queues.refetch()} />}
      <Tabs value={tab} onValueChange={(value) => setTab(value as "mine" | "queue")}>
        <TabsList>
          <TabsTrigger value="mine">{t("helpdesk.tabs.mine")}</TabsTrigger>
          {(agent || admin) && <TabsTrigger value="queue">{t("helpdesk.tabs.queue")}</TabsTrigger>}
        </TabsList>
        <TabsContent value="mine" className="mt-4">
          <TicketList role="mine" queues={items} />
        </TabsContent>
        {(agent || admin) && (
          <TabsContent value="queue" className="mt-4">
            <TicketList role="queue" queues={items} />
          </TabsContent>
        )}
      </Tabs>
      <NewTicketDialog open={creating} onOpenChange={setCreating} queues={items} />
    </Page>
  );
}
