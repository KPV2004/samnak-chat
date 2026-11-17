import type { ApprovalAttachment, ApprovalRequestDetail } from "@samnak/core";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link, useParams } from "react-router";
import { FormError } from "../components/FormError";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT, type T } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSessionState } from "../state/session";
import { ApprovalActions } from "./ApprovalActions";
import { actionKeys, payloadRows, statusKeys, summarize, typeKeys } from "./format";
import { useApprovalApi } from "./api";
import { useNameOf } from "./useMembers";

/** One request with its fields, attachments, history and the decisions still open to me (APR-07). */
export function ApprovalDetail() {
  return <RequireWorkspace>{() => <RequestView />}</RequireWorkspace>;
}

function RequestView() {
  const t = useT();
  const api = useApprovalApi();
  const { workspaceId } = useSessionState();
  const { requestId = "" } = useParams();
  const request = useQuery({
    queryKey: ["approval", workspaceId, "request", requestId],
    queryFn: () => api.getRequest(requestId),
    enabled: workspaceId !== null && requestId !== "",
  });

  if (request.isPending) return <p role="status">{t("loading")}</p>;
  if (request.isError) {
    return (
      <section>
        <BackLink />
        <FormError message={t(errorKey(request.error, { 404: "approval.notFound" }))} />
      </section>
    );
  }
  return <Detail request={request.data} />;
}

function Detail({ request: r }: { request: ApprovalRequestDetail }) {
  const t = useT();
  const locale = useCurrentLocale();
  const timeZone = useMe().data?.timezone;
  const nameOf = useNameOf();
  const when = (iso: string) => formatDateTime(iso, locale, timeZone);

  return (
    <section aria-labelledby="request-heading">
      <BackLink />
      <h1 id="request-heading">{summarize(r.type, r.payload, t)}</h1>
      <p>
        <span className="badge">{t(statusKeys[r.status])}</span> <small className="muted">{t(typeKeys[r.type])}</small>
      </p>

      <dl>
        <dt>{t("approval.requester")}</dt>
        <dd>{nameOf(r.requester_id)}</dd>
        <dt>{t("approval.approver")}</dt>
        <dd>{nameOf(r.approver_id)}</dd>
        <dt>{t("approval.sentOn")}</dt>
        <dd>{when(r.created_at)}</dd>
        {r.decided_at && (
          <>
            <dt>{t("approval.decidedOn")}</dt>
            <dd>{when(r.decided_at)}</dd>
          </>
        )}
        {r.status === "rejected" && r.reason && (
          <>
            <dt>{t("approval.rejectedReason")}</dt>
            <dd>{r.reason}</dd>
          </>
        )}
        {payloadRows(r.type, r.payload, t).map((row) => (
          <FieldPair key={row.label} label={row.label} value={row.value} multiline={row.multiline} />
        ))}
      </dl>

      <ApprovalActions requestId={r.id} requesterId={r.requester_id} approverId={r.approver_id} status={r.status} />

      {r.attachments.length > 0 && (
        <section aria-labelledby="attachments-heading">
          <h2 id="attachments-heading">{t("approval.attachments")}</h2>
          <ul className="rows">
            {r.attachments.map((a) => (
              <li key={a.id}>
                <Download attachment={a} t={t} />
              </li>
            ))}
          </ul>
        </section>
      )}

      <section aria-labelledby="history-heading">
        <h2 id="history-heading">{t("approval.history")}</h2>
        <ol aria-label={t("approval.history")}>
          {r.history.map((h) => (
            <li key={h.id}>
              <span>{t(actionKeys[h.action], { name: nameOf(h.actor_id) })}</span>{" "}
              <time dateTime={h.at}>
                <small className="muted">{when(h.at)}</small>
              </time>
              {h.reason && <p>{h.reason}</p>}
            </li>
          ))}
        </ol>
      </section>
    </section>
  );
}

function FieldPair({ label, value, multiline }: { label: string; value: string; multiline?: boolean }) {
  return (
    <>
      <dt>{label}</dt>
      <dd style={multiline ? { whiteSpace: "pre-wrap" } : undefined}>{value}</dd>
    </>
  );
}

/** Asks for a fresh download link when clicked: the stored URL would expire (APR-09). */
function Download({ attachment, t }: { attachment: ApprovalAttachment; t: T }) {
  const api = useApprovalApi();
  const download = useMutation({
    mutationFn: () => api.getAttachmentUrl(attachment.id),
    onSuccess: ({ url }) => {
      window.open(url, "_blank", "noopener");
    },
  });
  return (
    <>
      <button
        type="button"
        aria-label={t("approval.download", { name: attachment.file_name })}
        disabled={download.isPending}
        onClick={() => download.mutate()}
      >
          {attachment.file_name}
      </button>
      <FormError message={download.error ? t(errorKey(download.error)) : null} />
    </>
  );
}

function BackLink() {
  const t = useT();
  return (
    <p>
      <Link to="/approvals">{t("approval.backToList")}</Link>
    </p>
  );
}
