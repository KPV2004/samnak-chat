import type { ApprovalAttachment, ApprovalRequestDetail } from "@samnak/core";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowLeft, Download as DownloadIcon } from "lucide-react";
import { Link, useParams } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT, type T } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSessionState } from "../state/session";
import { ApprovalActions } from "./ApprovalActions";
import { actionKeys, historyDotTones, payloadRows, statusKeys, statusTones, summarize, typeKeys } from "./format";
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

  if (request.isPending) {
    return (
      <Page title={t("approval.title")} actions={<BackLink />}>
        <div role="status" className="grid gap-4">
          <span className="sr-only">{t("loading")}</span>
          <Skeleton className="h-40 w-full rounded-xl" />
          <Skeleton className="h-32 w-full rounded-xl" />
        </div>
      </Page>
    );
  }
  if (request.isError) {
    return (
      <Page title={t("approval.title")} actions={<BackLink />}>
        <FormError message={t(errorKey(request.error, { 404: "approval.notFound" }))} />
      </Page>
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
  const tone = statusTones[r.status];

  return (
    <Page
      title={summarize(r.type, r.payload, t)}
      description={
        <span className="inline-flex flex-wrap items-center gap-2">
          <Badge variant="outline" className={tone.text}>
            {t(statusKeys[r.status])}
          </Badge>
          <span>{t(typeKeys[r.type])}</span>
        </span>
      }
      actions={
        <>
          <BackLink />
          <ApprovalActions requestId={r.id} requesterId={r.requester_id} approverId={r.approver_id} status={r.status} />
        </>
      }
    >
      <Card className="gap-0 py-5">
        <CardContent>
          <dl className="grid gap-x-6 gap-y-4 sm:grid-cols-2">
            <Field label={t("approval.requester")} value={nameOf(r.requester_id)} />
            <Field label={t("approval.approver")} value={nameOf(r.approver_id)} />
            <Field label={t("approval.sentOn")} value={when(r.created_at)} />
            {r.decided_at && <Field label={t("approval.decidedOn")} value={when(r.decided_at)} />}
            {r.status === "rejected" && r.reason && <Field label={t("approval.rejectedReason")} value={r.reason} multiline />}
            {payloadRows(r.type, r.payload, t).map((row) => (
              <Field key={row.label} label={row.label} value={row.value} multiline={row.multiline} />
            ))}
          </dl>
        </CardContent>
      </Card>

      {r.attachments.length > 0 && (
        <Card className="gap-3 py-5">
          <CardHeader>
            <h2 id="attachments-heading" className="text-base font-semibold">
              {t("approval.attachments")}
            </h2>
          </CardHeader>
          <CardContent>
            <ul aria-labelledby="attachments-heading" className="m-0 grid list-none gap-2 p-0">
              {r.attachments.map((a) => (
                <li key={a.id}>
                  <Download attachment={a} t={t} />
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      )}

      <Card className="gap-3 py-5">
        <CardHeader>
          <h2 id="history-heading" className="text-base font-semibold">
            {t("approval.history")}
          </h2>
        </CardHeader>
        <CardContent>
          <ol aria-label={t("approval.history")} className="m-0 ml-1.5 grid list-none gap-5 border-l p-0 pl-6">
            {r.history.map((h) => (
              <li key={h.id} className="relative grid gap-0.5">
                <span
                  aria-hidden="true"
                  className={cn(
                    "absolute -left-[30px] top-1.5 size-2.5 rounded-full ring-4 ring-card",
                    historyDotTones[h.action],
                  )}
                />
                <p className="m-0 text-sm">{t(actionKeys[h.action], { name: nameOf(h.actor_id) })}</p>
                <time dateTime={h.at} className="text-xs text-muted-foreground">
                  {when(h.at)}
                </time>
                {h.reason && <p className="m-0 text-sm text-muted-foreground">{h.reason}</p>}
              </li>
            ))}
          </ol>
        </CardContent>
      </Card>
    </Page>
  );
}

/** A label above its value. Long text keeps its line breaks. */
function Field({ label, value, multiline }: { label: string; value: string; multiline?: boolean }) {
  return (
    <div className="grid content-start gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className={cn("m-0 text-sm break-words", multiline && "whitespace-pre-wrap")}>{value}</dd>
    </div>
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
    <div className="grid gap-2">
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="justify-self-start"
        aria-label={t("approval.download", { name: attachment.file_name })}
        disabled={download.isPending}
        onClick={() => download.mutate()}
      >
        <DownloadIcon />
        {attachment.file_name}
      </Button>
      <FormError message={download.error ? t(errorKey(download.error)) : null} />
    </div>
  );
}

function BackLink() {
  const t = useT();
  return (
    <Button asChild variant="ghost" size="sm">
      <Link to="/approvals">
        <ArrowLeft />
        {t("approval.backToList")}
      </Link>
    </Button>
  );
}
