import { useT } from "../i18n/useT";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { NewRequestForm } from "./NewRequestForm";
import { RequestList } from "./RequestList";

/** The Approvals page: a new request, then my requests and the ones waiting for me (APR-06). */
export function Approvals() {
  const t = useT();
  return (
    <RequireWorkspace>
      {() => (
        <section aria-labelledby="approvals-heading">
          <h1 id="approvals-heading">{t("approval.title")}</h1>
          <NewRequestForm />
          <RequestList role="requester" title={t("approval.mine")} empty={t("approval.noneMine")} />
          <RequestList role="approver" title={t("approval.toApprove")} empty={t("approval.noneToApprove")} />
        </section>
      )}
    </RequireWorkspace>
  );
}
