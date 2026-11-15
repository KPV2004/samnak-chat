import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useParams } from "react-router";
import { FormError } from "../components/FormError";
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
      navigate("/", { replace: true });
    },
  });

  return (
    <section aria-labelledby="accept-heading">
      <h1 id="accept-heading">{t("accept.title")}</h1>
      <p>{t("accept.body")}</p>
      <FormError
        message={
          accept.isError
            ? t(errorKey(accept.error, { 404: "accept.notFound", 403: "accept.wrongEmail", 409: "accept.alreadyMember" }))
            : null
        }
      />
      <button type="button" disabled={accept.isPending} onClick={() => accept.mutate()}>
        {t("accept.join")}
      </button>
    </section>
  );
}
