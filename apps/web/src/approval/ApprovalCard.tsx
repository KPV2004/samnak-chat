import { parseApprovalCard, type Message } from "@samnak/core";
import { Link } from "react-router";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { ApprovalActions } from "./ApprovalActions";
import { statusKeys, typeKeys } from "./format";
import { useModuleEnabled } from "./useModule";

/**
 * The renderer of `approval.request` cards in a chat (APR-03). While `/meta` lacks
 * approval, a card shows its fallback text like any card this client does not know (PLT-11).
 */
export function ApprovalCardGate({ message }: { message: Message }) {
  const { enabled } = useModuleEnabled("approval");
  return enabled ? <ApprovalCard message={message} /> : <FallbackCard message={message} />;
}

function FallbackCard({ message }: { message: Message }) {
  const t = useT();
  return (
    <div className="chat-card">
      <small className="muted">{t("chat.card")}</small>
      <div>{message.fallback_text || message.body}</div>
    </div>
  );
}

function ApprovalCard({ message }: { message: Message }) {
  const t = useT();
  const locale = useCurrentLocale();
  const timeZone = useMe().data?.timezone;
  const card = parseApprovalCard(message.payload);
  if (!card) return <FallbackCard message={message} />;

  return (
    <article className="chat-card approval-card">
      <small className="muted">
        {t("approval.cardKind")} · {t(typeKeys[card.type])}
      </small>
      <p>
        <strong>{card.summary}</strong>{" "}
        <span className="badge">{t(statusKeys[card.status])}</span>
      </p>
      <p className="muted">
        {t("approval.from", { name: card.requester_name })} · {t("approval.to", { name: card.approver_name })}
      </p>
      {card.status === "rejected" && card.reason && <p>{t("approval.reasonOf", { reason: card.reason })}</p>}
      {card.decided_at && (
        <small className="muted">{t("approval.decidedAt", { time: formatDateTime(card.decided_at, locale, timeZone) })}</small>
      )}
      <ApprovalActions
        requestId={card.request_id}
        requesterId={card.requester_id}
        approverId={card.approver_id}
        status={card.status}
      />
      <p>
        <Link to={`/approvals/${card.request_id}`}>{t("approval.open")}</Link>
      </p>
    </article>
  );
}
