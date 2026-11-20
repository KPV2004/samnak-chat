import { Plus } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { NewRequestForm } from "./NewRequestForm";
import { RequestList } from "./RequestList";

/**
 * The Approvals page: my requests and the ones waiting for me (APR-06). A new request is a
 * dialog opened from the page's primary button, so the lists stay in view.
 */
export function Approvals() {
  const t = useT();
  const [creating, setCreating] = useState(false);
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
          <RequestList role="requester" title={t("approval.mine")} empty={t("approval.noneMine")} />
          <RequestList role="approver" title={t("approval.toApprove")} empty={t("approval.noneToApprove")} />
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
