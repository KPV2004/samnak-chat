import { Plus, Settings } from "lucide-react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AccountLoadError } from "../components/AccountStates";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { NewTicketDialog } from "./NewTicketDialog";
import { TicketList } from "./TicketList";
import { helpdeskPath, helpdeskViewOf, useHelpdeskQueues, useHelpdeskRole } from "./view";

/**
 * The Helpdesk page (HD-01, HD-10): "My requests" for everyone, and the queue
 * list for agents and admins, chosen by `?view=` and `?queue=` from the sidebar. A ticket is
 * opened from the page's primary button or by `?new=1`; settings are for owners and admins.
 */
export function Helpdesk() {
  return <RequireWorkspace>{() => <HelpdeskView />}</RequireWorkspace>;
}

function HelpdeskView() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const queues = useHelpdeskQueues();
  const items = queues.data?.items ?? [];
  const { admin, agent } = useHelpdeskRole(items);
  const canQueue = agent || admin;
  // The list, the queue and the new-ticket dialog are in the URL, so the sidebar's links drive them.
  // A queue list asked for by someone who has none shows "My requests" once the queues are known.
  const asked = helpdeskViewOf(params.get("view"));
  const tab = asked === "queue" && (canQueue || queues.isPending) ? "queue" : "mine";
  const queueId = params.get("queue") ?? "";
  const creating = params.get("new") === "1";

  function setCreating(open: boolean) {
    const next = new URLSearchParams(params);
    if (open) next.set("new", "1");
    else next.delete("new");
    setParams(next, { replace: true });
  }

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
      <Tabs value={tab} onValueChange={(value) => navigate(helpdeskPath(value as "mine" | "queue"))}>
        <TabsList>
          <TabsTrigger value="mine">{t("helpdesk.tabs.mine")}</TabsTrigger>
          {canQueue && <TabsTrigger value="queue">{t("helpdesk.tabs.queue")}</TabsTrigger>}
        </TabsList>
        <TabsContent value="mine" className="mt-4">
          <TicketList role="mine" queues={items} />
        </TabsContent>
        {canQueue && (
          <TabsContent value="queue" className="mt-4">
            <TicketList role="queue" queues={items} queueId={queueId} onQueueChange={(id) => navigate(helpdeskPath("queue", id), { replace: true })} />
          </TabsContent>
        )}
      </Tabs>
      <NewTicketDialog open={creating} onOpenChange={setCreating} queues={items} />
    </Page>
  );
}
