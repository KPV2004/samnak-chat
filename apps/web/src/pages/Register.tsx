import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useState } from "react";
import { Link, useLocation } from "react-router";
import { Button } from "@/components/ui/button";
import { AuthLayout } from "../components/AuthLayout";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { deviceName } from "../lib/device-name";
import { errorKey } from "../lib/errors";
import { emailPattern, focusField, readForm } from "../lib/form";
import { useSession } from "../state/session";

const fields = ["display_name", "email", "password"] as const;
type Errors = Partial<Record<(typeof fields)[number], MessageKey>>;

export function Register() {
  const t = useT();
  const session = useSession();
  const queryClient = useQueryClient();
  const location = useLocation();
  const [errors, setErrors] = useState<Errors>({});

  // The API registers without signing in, so sign in right after with the same credentials.
  const register = useMutation({
    mutationFn: async (v: { display_name: string; email: string; password: string }) => {
      await session.register(v);
      return session.login({ email: v.email, password: v.password, device_name: deviceName(), platform: "web" });
    },
    onSuccess: (pair) => queryClient.setQueryData(["me"], pair.user),
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const v = readForm(form);
    const next: Errors = {};
    if (!v.display_name?.trim()) next.display_name = "register.nameRequired";
    if (!emailPattern.test(v.email?.trim() ?? "")) next.email = "login.emailRequired";
    const pw = v.password ?? "";
    if (pw.length < 8 || pw.length > 128) next.password = "register.passwordLength";
    setErrors(next);
    const first = fields.find((k) => next[k]);
    if (first) return focusField(form, first);
    register.mutate({ display_name: v.display_name!.trim(), email: v.email!.trim(), password: pw });
  }

  return (
    <AuthLayout title={t("register.title")}>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("displayName")}
          name="display_name"
          autoComplete="name"
          error={errors.display_name && t(errors.display_name)}
        />
        <Field
          label={t("email")}
          name="email"
          type="email"
          autoComplete="username"
          error={errors.email && t(errors.email)}
        />
        <Field
          label={t("password")}
          name="password"
          type="password"
          autoComplete="new-password"
          hint={t("register.passwordHint")}
          error={errors.password && t(errors.password)}
        />
        <FormError message={register.isError ? t(errorKey(register.error, { 409: "register.emailTaken" })) : null} />
        <Button type="submit" className="w-full" disabled={register.isPending}>
          {register.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
          {t("register.submit")}
        </Button>
      </form>
      <p className="m-0 mt-4 text-center text-sm text-muted-foreground">
        <Link to="/login" state={location.state} className="font-medium text-primary underline-offset-4 hover:underline">
          {t("register.toLogin")}
        </Link>
      </p>
    </AuthLayout>
  );
}
