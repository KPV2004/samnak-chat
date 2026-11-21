import type { HelpdeskQueue } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { canManage, useMe, useWorkspaces } from "../state/queries";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";

/** The two lists of the Helpdesk app; `mine` is the default. */
export type HelpdeskView = "mine" | "queue";

export function helpdeskViewOf(raw: string | null): HelpdeskView {
  return raw === "queue" ? "queue" : "mine";
}

/** The `/helpdesk` URL of a list; `queueId` narrows the queue list to one queue. */
export function helpdeskPath(view: HelpdeskView, queueId = ""): string {
  if (view === "mine") return "/helpdesk";
  return queueId ? `/helpdesk?view=queue&queue=${encodeURIComponent(queueId)}` : "/helpdesk?view=queue";
}

/** The queues of the workspace, shared by the Helpdesk page and its sidebar (one request). */
export function useHelpdeskQueues(enabled = true) {
  const api = useHelpdeskApi();
  const { workspaceId } = useSessionState();
  return useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: enabled && workspaceId !== null,
  });
}

/** What the signed-in user may do in the Helpdesk: admins manage it; agents work the queues they are in. */
export function useHelpdeskRole(queues: HelpdeskQueue[]) {
  const me = useMe().data?.id;
  const workspace = useWorkspaces().current;
  const admin = canManage(workspace?.role);
  const agentOf = me === undefined ? [] : queues.filter((q) => q.agent_ids.includes(me));
  return { admin, agent: agentOf.length > 0, agentOf };
}
