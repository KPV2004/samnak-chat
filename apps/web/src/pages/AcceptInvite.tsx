import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useNavigate, useParams } from "react-router";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useSession } from "../state/session";

export function AcceptInvite() {
  const t = useT();
  const { token = "" } = useParams();
  const session = useSession();
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const accept = useMutation({
    mutationFn: () => session.api.acceptInvite(token),
    onSuccess: (workspace) => {
      queryClient.setQueryData<unknown[]>(["workspaces"], (old) => [
        ...(old ?? []).filter((w) => (w as { id: string }).id !== workspace.id),
        workspace,
      ]);
      session.setWorkspaceId(workspace.id);
      toast.success(t("accept.joined", { name: workspace.name }));
      navigate("/", { replace: true });
    },
  });

  return (
    <Page width="narrow" title={t("accept.title")} description={t("accept.body")}>
      <div className="grid justify-items-start gap-4">
        <FormError
          message={
            accept.isError
              ? t(errorKey(accept.error, { 404: "accept.notFound", 403: "accept.wrongEmail", 409: "accept.alreadyMember" }))
              : null
          }
        />
        <Button type="button" disabled={accept.isPending} onClick={() => accept.mutate()}>
          {accept.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
          {t("accept.join")}
        </Button>
      </div>
    </Page>
  );
}
