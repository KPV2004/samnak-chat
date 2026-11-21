import { Inbox } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { DataList, DataRow, ListLoading, LoadMore, stretchedLink } from "../pages/module-ui";
import { useMe } from "../state/queries";
import { statusKeys, statusTones, summarize } from "./format";
import { useApprovalRequests, type RequestRole } from "./requests";
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
  const items = requests.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <section aria-labelledby={headingId} className="grid gap-3">
      <h2 id={headingId} className="text-base font-semibold">
        {title}
      </h2>
      {requests.isPending && <ListLoading rows={2} />}
      {requests.isError && <AccountLoadError message={t(errorKey(requests.error))} onRetry={() => void requests.refetch()} />}
      {requests.data && items.length === 0 && <EmptyState icon={Inbox} title={empty} />}
      {items.length > 0 && (
        <DataList aria-label={title}>
          {items.map((r) => (
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
              <Badge variant="outline" className={cn("shrink-0", statusTones[r.status].text)}>
                {t(statusKeys[r.status])}
              </Badge>
            </DataRow>
          ))}
        </DataList>
      )}
      {requests.hasNextPage && <LoadMore loading={requests.isFetchingNextPage} onClick={() => void requests.fetchNextPage()} />}
    </section>
  );
}
