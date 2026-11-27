import type { ApprovalRequest } from "@samnak/core";
import { Inbox } from "lucide-react";
import { useId, useState } from "react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { AccountLoadError } from "../components/AccountStates";
import { ListSearch, matchesQuery } from "../components/ListSearch";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { DataList, DataRow, ListLoading, LoadMore, stretchedLink } from "../pages/module-ui";
import { useMe } from "../state/queries";
import { statusKeys, statusTones, summarize } from "./format";
import { RejectRequestDialog } from "./RejectRequestDialog";
import { useApprovalRequests, type RequestRole } from "./requests";
import { useQuickApprove } from "./useQuickApprove";
import { useNameOf } from "./useMembers";

interface Props {
  role: RequestRole;
  title: string;
  empty: string;
}

/** The list of the Approvals page for one view, read a page at a time. */
export function RequestList({ role, title, empty }: Props) {
  const t = useT();
  const locale = useCurrentLocale();
  const headingId = useId();
  const timeZone = useMe().data?.timezone;
  const nameOf = useNameOf();

  const requests = useApprovalRequests(role);
  const [query, setQuery] = useState("");
  const quickApprove = useQuickApprove();
  const [rejecting, setRejecting] = useState<ApprovalRequest | null>(null);
  const items = requests.data?.pages.flatMap((p) => p.items) ?? [];
  const shown = items.filter((r) =>
    matchesQuery(query, summarize(r.type, r.payload, t), r.type, role === "requester" ? nameOf(r.approver_id) : nameOf(r.requester_id)),
  );

  return (
    <section aria-labelledby={headingId} className="grid gap-3">
      <h2 id={headingId} className="text-base font-semibold">
        {title}
      </h2>
      {requests.isPending && <ListLoading rows={2} />}
      {requests.isError && <AccountLoadError message={t(errorKey(requests.error))} onRetry={() => void requests.refetch()} />}
      {items.length > 0 && <ListSearch value={query} onChange={setQuery} count={items.length} />}
      {requests.data && items.length === 0 && <EmptyState icon={Inbox} title={empty} />}
      {items.length > 0 && shown.length === 0 && (
        <p role="status" className="m-0 text-sm text-muted-foreground">
          {t("list.search.empty", { n: items.length })}{" "}
          <Button type="button" variant="link" size="sm" onClick={() => setQuery("")}>
            {t("list.search.clear")}
          </Button>
        </p>
      )}
      {shown.length > 0 && (
        <DataList aria-label={title}>
          {shown.map((r) => (
            <DataRow key={r.id} interactive className="flex-nowrap items-start">
              <div className="grid min-w-0 gap-0.5">
                <Link to={`/approvals/${r.id}`} className={cn("font-medium break-words", stretchedLink)}>
                  {summarize(r.type, r.payload, t)}
                </Link>
                <p className="m-0 text-xs text-muted-foreground">
                  {role === "requester"
                    ? t("approval.to", { name: nameOf(r.approver_id) })
                    : t("approval.from", { name: nameOf(r.requester_id) })}
                  {" · "}
                  {t("approval.sentAt", { time: formatDateTime(r.created_at, locale, timeZone) })}
                </p>
              </div>
              <div className="relative z-10 flex shrink-0 items-center gap-1">
                {role === "approver" && r.status === "pending" && (
                  <>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      onClick={() => quickApprove(r)}
                      aria-label={`${t("approval.approve")} ${summarize(r.type, r.payload, t)}`}
                    >
                      {t("approval.approve")}
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => setRejecting(r)}
                      aria-label={`${t("approval.reject")} ${summarize(r.type, r.payload, t)}`}
                    >
                      {t("approval.reject")}
                    </Button>
                  </>
                )}
                <Badge variant="outline" className={cn("shrink-0", statusTones[r.status].text)}>
                  {t(statusKeys[r.status])}
                </Badge>
              </div>
            </DataRow>
          ))}
        </DataList>
      )}
      {requests.hasNextPage && <LoadMore loading={requests.isFetchingNextPage} onClick={() => void requests.fetchNextPage()} />}
      <RejectRequestDialog requestId={rejecting?.id ?? null} onClose={() => setRejecting(null)} />
    </section>
  );
}
