import { parseApprovalCard, type Message } from "@samnak/core";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { Link } from "react-router";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { CardFrame, FallbackCard } from "../pages/module-ui";
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

/** A compact Slack-style attachment: a coloured bar for the status, the summary, who it is between, and the actions. */
function ApprovalCard({ message }: { message: Message }) {
  const t = useT();
  const locale = useCurrentLocale();
  const timeZone = useMe().data?.timezone;
  const card = parseApprovalCard(message.payload);
  if (!card) return <FallbackCard message={message} />;
  const tone = statusTones[card.status];

  return (
    <CardFrame
      kind={
        <>
          {t("approval.cardKind")} · {t(typeKeys[card.type])}
        </>
      }
      bar={tone.bar}
      badge={
        <Badge variant="outline" className={cn("shrink-0", tone.text)}>
          {t(statusKeys[card.status])}
        </Badge>
      }
      title={card.summary}
      meta={
        <>
          <p className="m-0">
            {t("approval.from", { name: card.requester_name })} · {t("approval.to", { name: card.approver_name })}
          </p>
          {card.decided_at && <p className="m-0">{t("approval.decidedAt", { time: formatDateTime(card.decided_at, locale, timeZone) })}</p>}
        </>
      }
      actions={
        <>
          <ApprovalActions
            size="sm"
            requestId={card.request_id}
            requesterId={card.requester_id}
            approverId={card.approver_id}
            status={card.status}
          />
          <Button asChild variant="ghost" size="sm" className="ml-auto">
            <Link to={`/approvals/${card.request_id}`}>{t("approval.open")}</Link>
          </Button>
        </>
      }
    >
      {card.status === "rejected" && card.reason && <p className="m-0">{t("approval.reasonOf", { reason: card.reason })}</p>}
    </CardFrame>
  );
}
