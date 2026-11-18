import { parseApprovalCard, type Message } from "@samnak/core";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Link } from "react-router";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { ApprovalActions } from "./ApprovalActions";
import { statusKeys, statusTones, typeKeys } from "./format";
import { useModuleEnabled } from "../modules";

/**
 * The renderer of `approval.request` cards in a chat (APR-03). While `/meta` lacks
 * approval, a card shows its fallback text like any card this client does not know (PLT-11).
 */
export function ApprovalCardGate({ message }: { message: Message }) {
  const enabled = useModuleEnabled("approval");
  return enabled ? <ApprovalCard message={message} /> : <FallbackCard message={message} />;
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

/** A compact Slack-style attachment: a coloured bar for the status, the summary, who it is between, and the actions. */
function ApprovalCard({ message }: { message: Message }) {
  const t = useT();
  const locale = useCurrentLocale();
  const timeZone = useMe().data?.timezone;
  const card = parseApprovalCard(message.payload);
  if (!card) return <FallbackCard message={message} />;
  const tone = statusTones[card.status];

  return (
    <article className={cn("grid max-w-md gap-2 rounded-lg border border-l-[3px] bg-card p-3 text-sm text-card-foreground", tone.bar)}>
      <div className="flex items-start justify-between gap-2">
        <p className="m-0 text-xs text-muted-foreground">
          {t("approval.cardKind")} · {t(typeKeys[card.type])}
        </p>
        <Badge variant="outline" className={cn("shrink-0", tone.text)}>
          {t(statusKeys[card.status])}
        </Badge>
      </div>
      <div className="grid gap-0.5">
        <p className="m-0 font-medium">{card.summary}</p>
        <p className="m-0 text-xs text-muted-foreground">
          {t("approval.from", { name: card.requester_name })} · {t("approval.to", { name: card.approver_name })}
        </p>
      </div>
      {card.status === "rejected" && card.reason && <p className="m-0">{t("approval.reasonOf", { reason: card.reason })}</p>}
      {card.decided_at && (
        <p className="m-0 text-xs text-muted-foreground">
          {t("approval.decidedAt", { time: formatDateTime(card.decided_at, locale, timeZone) })}
        </p>
      )}
      <ApprovalActions
        size="sm"
        requestId={card.request_id}
        requesterId={card.requester_id}
        approverId={card.approver_id}
        status={card.status}
      />
      <div>
        <Button asChild variant="ghost" size="sm" className="-ml-3">
          <Link to={`/approvals/${card.request_id}`}>{t("approval.open")}</Link>
        </Button>
      </div>
    </article>
  );
}
