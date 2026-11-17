import { useInfiniteQuery } from "@tanstack/react-query";
import { useId } from "react";
import { Link } from "react-router";
import { FormError } from "../components/FormError";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { useMe } from "../state/queries";
import { useSessionState } from "../state/session";
import { useApprovalApi } from "./api";
import { statusKeys, summarize } from "./format";
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
    queryFn: ({ pageParam }) => api.listRequests({ role, cursor: pageParam || undefined, limit: 20 }),
    getNextPageParam: (page) => page.next_cursor || undefined,
    enabled: workspaceId !== null,
  });
  const items = requests.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <section aria-labelledby={headingId}>
      <h2 id={headingId}>{title}</h2>
      {requests.isPending && <p role="status">{t("loading")}</p>}
      {requests.isError && <FormError message={t(errorKey(requests.error))} />}
      {requests.data && items.length === 0 && <p>{empty}</p>}
      {items.length > 0 && (
        <ul className="rows" aria-label={title}>
          {items.map((r) => (
            <li key={r.id}>
              <div>
                <Link to={`/approvals/${r.id}`}>{summarize(r.type, r.payload, t)}</Link>{" "}
                <span className="badge">{t(statusKeys[r.status])}</span>
                <br />
                <small className="muted">
                  {role === "requester"
                    ? t("approval.to", { name: nameOf(r.approver_id) })
                    : t("approval.from", { name: nameOf(r.requester_id) })}
                  {" · "}
                  {t("approval.sentAt", { time: formatDateTime(r.created_at, locale, timeZone) })}
                </small>
              </div>
            </li>
          ))}
        </ul>
      )}
      {requests.hasNextPage && (
        <p>
          <button type="button" disabled={requests.isFetchingNextPage} onClick={() => void requests.fetchNextPage()}>
            {t("loadMore")}
          </button>
        </p>
      )}
    </section>
  );
}
