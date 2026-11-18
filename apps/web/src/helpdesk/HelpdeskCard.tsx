import type { Message } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useModuleEnabled } from "../modules";
import { useMe } from "../state/queries";
import { useSessionState } from "../state/session";
import { useHelpdeskApi } from "./api";
import { parseHelpdeskCard } from "./card";
import { priorityKeys, priorityTones, statusKeys, statusTones } from "./format";

/**
 * The renderer of `helpdesk.ticket` cards in a chat (HD-08). While `/meta` lacks helpdesk, a card
 * shows its fallback text like any card this client does not know (PLT-11).
 */
export function HelpdeskCardGate({ message }: { message: Message }) {
  const enabled = useModuleEnabled("helpdesk");
  return enabled ? <TicketCard message={message} /> : <FallbackCard message={message} />;
}

function FallbackCard({ message }: { message: Message }) {
  const t = useT();
  return (
    <div className="grid max-w-md gap-1 rounded-lg border bg-card p-3 text-sm text-card-foreground">
      <p className="m-0 text-xs text-muted-foreground">{t("chat.card")}</p>
      <p className="m-0">{message.fallback_text || message.body}</p>
    </div>
  );
}

/**
 * A ticket in a chat: the key, title, status and priority, the queue, requester and assignee, and
 * "Claim" for an agent of the queue while nobody has the ticket. Every change of the ticket arrives as
 * `card.updated`, which the chat store applies to the message, so the card re-renders by itself.
 */
function TicketCard({ message }: { message: Message }) {
  const t = useT();
  const api = useHelpdeskApi();
  const queryClient = useQueryClient();
  const { workspaceId } = useSessionState();
  const userId = useMe().data?.id ?? null;
  const card = parseHelpdeskCard(message.payload);
  const queues = useQuery({
    queryKey: ["helpdesk", "queues", workspaceId],
    queryFn: () => api.listQueues(),
    enabled: workspaceId !== null,
  });
  const claim = useMutation({
    mutationFn: (input: { ticketId: string; userId: string }) => api.assignTicket(input.ticketId, input.userId),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["helpdesk"] }),
  });

  if (!card) return <FallbackCard message={message} />;
  const tone = statusTones[card.status];
  const agentOfQueue = queues.data?.items.some((q) => q.id === card.can_claim_queue_id && userId !== null && q.agent_ids.includes(userId)) ?? false;
  const claimable = agentOfQueue && card.assignee_id === null && card.status !== "closed";

  return (
    <article className="grid max-w-md gap-2 rounded-lg border bg-card p-3 text-sm text-card-foreground">
      <div className="flex items-start justify-between gap-2">
        <p className="m-0 text-xs text-muted-foreground">
          {t("helpdesk.cardKind")} · <span className="font-mono">{card.key}</span>
        </p>
        <Badge variant={tone.badge} className={cn("shrink-0", tone.text)}>
          {t(statusKeys[card.status])}
        </Badge>
      </div>
      <div className="grid gap-0.5">
        <p className="m-0 font-medium">{card.title}</p>
        <p className="m-0 text-xs text-muted-foreground">{t("helpdesk.card.line", { queue: card.queue_name, requester: card.requester_name })}</p>
      </div>
      <p className="m-0 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <Badge variant={priorityTones[card.priority].badge}>{t(priorityKeys[card.priority])}</Badge>
        <span>
          {t("helpdesk.assignee")}: {card.assignee_id ? card.assignee_name : t("helpdesk.unassigned")}
        </span>
      </p>
      {claimable && userId !== null && (
        <div>
          <Button size="sm" disabled={claim.isPending} onClick={() => claim.mutate({ ticketId: card.ticket_id, userId })}>
            {t("helpdesk.detail.claim")}
          </Button>
        </div>
      )}
      <FormError message={claim.error ? t(errorKey(claim.error, { 409: "helpdesk.error.invalidTransition", 403: "helpdesk.error.notAllowed" })) : null} />
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-3">
          <Link to={`/helpdesk/${card.ticket_id}`}>{t("helpdesk.card.open")}</Link>
        </Button>
      </div>
    </article>
  );
}
