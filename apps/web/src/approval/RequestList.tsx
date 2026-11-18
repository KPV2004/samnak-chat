import { useInfiniteQuery } from "@tanstack/react-query";
import { Inbox } from "lucide-react";
import { useId } from "react";
import { Link } from "react-router";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSessionState } from "../state/session";
import { useApprovalApi } from "./api";
import { statusKeys, statusTones, summarize } from "./format";
import { useNameOf } from "./useMembers";

interface Props {
  /** requester: my requests; approver: waiting for me (APR-06). */
  role: "requester" | "approver";
  title: string;
  empty: string;
}

/** One of the two lists on the Approvals page, read a page at a time. */
export function RequestList({ role, title, empty }: Props) {
  const t = useT();
  const locale = useCurrentLocale();
  const api = useApprovalApi();
  const headingId = useId();
  const { workspaceId } = useSessionState();
  const timeZone = useMe().data?.timezone;
  const nameOf = useNameOf();

  const requests = useInfiniteQuery({
    queryKey: ["approval", workspaceId, "requests", role],
    initialPageParam: "",
    // "Waiting for me" holds only what still needs a decision; decided ones stay in the chat cards.
    queryFn: ({ pageParam }) =>
      api.listRequests({ role, status: role === "approver" ? "pending" : undefined, cursor: pageParam || undefined, limit: 20 }),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: workspaceId !== null,
  });
  const items = requests.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <section aria-labelledby={headingId} className="grid gap-3">
      <h2 id={headingId} className="text-base font-semibold">
        {title}
      </h2>
      {requests.isPending && (
        <div role="status" className="grid gap-2 rounded-lg border bg-card p-4">
          <span className="sr-only">{t("loading")}</span>
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      )}
      {requests.isError && (
        <div role="alert" className="flex flex-wrap items-center gap-2 text-sm text-destructive">
          <p className="m-0">{t(errorKey(requests.error))}</p>
          <Button type="button" variant="outline" size="sm" onClick={() => void requests.refetch()}>
            {t("retry")}
          </Button>
        </div>
      )}
      {requests.data && items.length === 0 && <EmptyState icon={Inbox} title={empty} />}
      {items.length > 0 && (
        <ul aria-label={title} className="divide-y rounded-lg border bg-card">
          {items.map((r) => (
            <li key={r.id} className="flex items-start justify-between gap-3 px-4 py-3">
              <div className="grid min-w-0 gap-0.5">
                <Link to={`/approvals/${r.id}`} className="font-medium break-words underline-offset-4 hover:underline">
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
            </li>
          ))}
        </ul>
      )}
      {requests.hasNextPage && (
        <div className="flex justify-center">
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={requests.isFetchingNextPage}
            onClick={() => void requests.fetchNextPage()}
          >
            {t("loadMore")}
          </Button>
        </div>
      )}
    </section>
  );
}
