import { useMutation } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { AuthLayout } from "../components/AuthLayout";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { emailPattern, focusField, readForm } from "../lib/form";
import { useSession } from "../state/session";

/** Asks for a reset link. The answer is the same whether or not the address has an account. */
export function ForgotPassword() {
  const t = useT();
  const session = useSession();
  const [emailError, setEmailError] = useState<MessageKey>();
  const forgot = useMutation({
    mutationFn: (email: string) => session.api.forgotPassword(email),
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const email = readForm(form).email?.trim() ?? "";
    if (!emailPattern.test(email)) {
      setEmailError("login.emailRequired");
      return focusField(form, "email");
    }
    setEmailError(undefined);
    forgot.mutate(email);
  }

  const back = (
    <p className="m-0 mt-4 text-center text-sm text-muted-foreground">
      <Link to="/login" className="font-medium text-primary underline-offset-4 hover:underline">
        {t("forgot.toLogin")}
      </Link>
    </p>
  );

  if (forgot.isSuccess) {
    return (
      <AuthLayout title={t("forgot.sentTitle")}>
        <p className="m-0 text-sm">{t("forgot.sent")}</p>
        {back}
      </AuthLayout>
    );
  }

  return (
    <AuthLayout title={t("forgot.title")}>
      <p className="m-0 mb-4 text-sm text-muted-foreground">{t("forgot.body")}</p>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("email")}
          name="email"
          type="email"
          autoComplete="username"
          error={emailError && t(emailError)}
        />
        <FormError message={forgot.isError ? t(errorKey(forgot.error)) : null} />
        <Button type="submit" className="w-full" disabled={forgot.isPending}>
          {forgot.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
          {t("forgot.submit")}
        </Button>
      </form>
      {back}
    </AuthLayout>
  );
}
