import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "@samnak/core";
import { useEffect, useRef } from "react";
import { Link, useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { AccountLoadError, AccountLoading } from "../components/AccountStates";
import { AuthLayout } from "../components/AuthLayout";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { useSession } from "../state/session";

/** The link from the verification mail. It works signed in or out, and confirms the address once. */
export function VerifyEmail() {
  const t = useT();
  const session = useSession();
  const queryClient = useQueryClient();
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const verify = useMutation({
    mutationFn: (value: string) => session.api.verifyEmail(value),
    // A signed-in profile still says "not verified" until it is fetched again.
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["me"] });
    },
  });
  const { mutate } = verify;

  // The link works once, and React runs effects twice in development: the ref keeps the second run from
  // spending the same token again.
  const started = useRef<string | null>(null);
  useEffect(() => {
    if (!token || started.current === token) return;
    started.current = token;
    mutate(token);
  }, [token, mutate]);

  const refused = !token || (verify.error instanceof ApiError && verify.error.status === 400);

  let body;
  if (refused) {
    body = (
      <div className="grid gap-4">
        <p className="m-0 text-sm">{t("verify.invalid")}</p>
        <Button asChild variant="outline" className="w-full">
          <Link to="/login">{t("verify.signIn")}</Link>
        </Button>
      </div>
    );
  } else if (verify.isError) {
    body = <AccountLoadError message={t(errorKey(verify.error))} onRetry={() => mutate(token)} />;
  } else if (verify.isSuccess) {
    body = (
      <div className="grid gap-4">
        <p className="m-0 text-sm">{t("verify.done")}</p>
        <Button asChild className="w-full">
          <Link to="/">{t("verify.continue")}</Link>
        </Button>
      </div>
    );
  } else {
    body = (
      <AccountLoading>
        <p className="m-0 text-sm text-muted-foreground">{t("verify.pending")}</p>
      </AccountLoading>
    );
  }

  return (
    <AuthLayout title={t("verify.title")} redirectSignedIn={false}>
      {body}
    </AuthLayout>
  );
}
