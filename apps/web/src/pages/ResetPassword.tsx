import { useMutation } from "@tanstack/react-query";
import { ApiError } from "@samnak/core";
import { Loader2Icon } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { Button } from "@/components/ui/button";
import { AuthLayout } from "../components/AuthLayout";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";
import { useSession } from "../state/session";

/** Sets a new password from the link in the reset mail. It works signed in or out. */
export function ResetPassword() {
  const t = useT();
  const session = useSession();
  const [params] = useSearchParams();
  const token = params.get("token") ?? "";
  const [passwordError, setPasswordError] = useState<MessageKey>();
  const reset = useMutation({
    mutationFn: (v: { token: string; password: string }) => session.api.resetPassword(v),
    onSuccess: () => {
      // The reset ends every session, this browser's included; clear the local one now.
      if (session.getState().signedIn) void session.logout(false);
    },
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const password = readForm(form).password ?? "";
    if (password.length < 8 || password.length > 128) {
      setPasswordError("register.passwordLength");
      return focusField(form, "password");
    }
    setPasswordError(undefined);
    reset.mutate({ token, password });
  }

  if (reset.isSuccess) {
    return (
      <AuthLayout title={t("reset.doneTitle")} redirectSignedIn={false}>
        <div className="grid gap-4">
          <p className="m-0 text-sm">{t("reset.doneBody")}</p>
          <Button asChild className="w-full">
            <Link to="/login">{t("reset.signIn")}</Link>
          </Button>
        </div>
      </AuthLayout>
    );
  }

  // A used, expired or unknown link gets the same answer from the server.
  const refused = !token || (reset.error instanceof ApiError && reset.error.status === 400);
  if (refused) {
    return (
      <AuthLayout title={t("reset.title")} redirectSignedIn={false}>
        <div className="grid gap-4">
          <p className="m-0 text-sm">{t("reset.invalid")}</p>
          <Button asChild variant="outline" className="w-full">
            <Link to="/forgot-password">{t("reset.newLink")}</Link>
          </Button>
        </div>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title={t("reset.title")} redirectSignedIn={false}>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("password")}
          name="password"
          type="password"
          autoComplete="new-password"
          hint={t("register.passwordHint")}
          error={passwordError && t(passwordError)}
        />
        <FormError message={reset.isError ? t(errorKey(reset.error)) : null} />
        <Button type="submit" className="w-full" disabled={reset.isPending}>
          {reset.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
          {t("reset.submit")}
        </Button>
      </form>
    </AuthLayout>
  );
}
