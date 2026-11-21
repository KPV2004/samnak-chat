import { Plus } from "lucide-react";
import { useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { NewRequestForm } from "./NewRequestForm";
import { RequestList } from "./RequestList";
import { approvalViewOf, viewRole } from "./requests";

/**
 * The Approvals page (APR-06): one list at a time, the requests waiting for me or my own, chosen by
 * `?view=` from the sidebar. A new request is a dialog opened from the page's primary button or by
 * `?new=1`, so the list stays in view.
 */
export function Approvals() {
  const t = useT();
  const [params, setParams] = useSearchParams();
  const view = approvalViewOf(params.get("view"));
  // "New request" in the sidebar links here with ?new=1, so the dialog is a URL too.
  const creating = params.get("new") === "1";
  function setCreating(open: boolean) {
    const next = new URLSearchParams(params);
    if (open) next.set("new", "1");
    else next.delete("new");
    setParams(next, { replace: true });
  }
  return (
    <RequireWorkspace>
      {() => (
        <Page
          title={t("approval.title")}
          width="wide"
          actions={
            <Button type="button" onClick={() => setCreating(true)}>
              <Plus />
              {t("approval.new")}
            </Button>
          }
        >
          {view === "mine" ? (
            <RequestList role={viewRole.mine} title={t("approval.mine")} empty={t("approval.noneMine")} />
          ) : (
            <RequestList role={viewRole.waiting} title={t("approval.toApprove")} empty={t("approval.noneToApprove")} />
          )}
          <Dialog open={creating} onOpenChange={setCreating}>
            <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
              <DialogHeader>
                <DialogTitle>{t("approval.new")}</DialogTitle>
                <DialogDescription>{t("approval.newHint")}</DialogDescription>
              </DialogHeader>
              <NewRequestForm onDone={() => setCreating(false)} />
            </DialogContent>
          </Dialog>
        </Page>
      )}
    </RequireWorkspace>
  );
}
