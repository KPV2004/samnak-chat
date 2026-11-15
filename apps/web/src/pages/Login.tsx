import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Link, useLocation } from "react-router";
import { AuthLayout } from "../components/AuthLayout";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { deviceName } from "../lib/device-name";
import { errorKey } from "../lib/errors";
import { emailPattern, focusField, readForm } from "../lib/form";
import { useLocale } from "../state/locale";
import { useSession } from "../state/session";
import type { MessageKey } from "../i18n/messages";

type Errors = Partial<Record<"email" | "password", MessageKey>>;

export function Login() {
  const t = useT();
  const session = useSession();
  const queryClient = useQueryClient();
  const setLocale = useLocale((s) => s.setLocale);
  const location = useLocation();
  const [errors, setErrors] = useState<Errors>({});

  const login = useMutation({
    mutationFn: (v: { email: string; password: string }) =>
      session.login({ ...v, device_name: deviceName(), platform: "web" }),
    onSuccess: (pair) => {
      queryClient.setQueryData(["me"], pair.user);
      setLocale(pair.user.locale);
    },
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const v = readForm(form);
    const next: Errors = {};
    if (!emailPattern.test(v.email?.trim() ?? "")) next.email = "login.emailRequired";
    if (!v.password) next.password = "login.passwordRequired";
    setErrors(next);
    const first = (["email", "password"] as const).find((k) => next[k]);
    if (first) return focusField(form, first);
    login.mutate({ email: v.email!.trim(), password: v.password! });
  }

  return (
    <AuthLayout title={t("login.title")}>
      <form onSubmit={onSubmit} noValidate>
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
          autoComplete="current-password"
          error={errors.password && t(errors.password)}
        />
        <FormError
          message={login.isError ? t(errorKey(login.error, { 401: "login.wrong", 429: "login.rateLimited" })) : null}
        />
        <button type="submit" disabled={login.isPending}>
          {t("login.submit")}
        </button>
      </form>
      <p>
        <Link to="/register" state={location.state}>
          {t("login.toRegister")}
        </Link>
      </p>
    </AuthLayout>
  );
}
