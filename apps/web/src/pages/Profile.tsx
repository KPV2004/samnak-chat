import type { Locale, User } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import type { MessageKey } from "../i18n/messages";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";
import { useLocale } from "../state/locale";
import { useMe } from "../state/queries";
import { useSession } from "../state/session";

function validTimeZone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

const zones: string[] = (Intl as unknown as { supportedValuesOf?: (key: string) => string[] }).supportedValuesOf?.("timeZone") ?? [];

export function Profile() {
  const t = useT();
  const me = useMe();
  return (
    <section aria-labelledby="profile-heading">
      <h1 id="profile-heading">{t("profile.title")}</h1>
      {me.isPending && <p role="status">{t("loading")}</p>}
      {me.isError && <FormError message={t(errorKey(me.error))} />}
      {me.data && <ProfileForm user={me.data} />}
    </section>
  );
}

function ProfileForm({ user }: { user: User }) {
  const t = useT();
  const session = useSession();
  const queryClient = useQueryClient();
  const setLocale = useLocale((s) => s.setLocale);
  const [errors, setErrors] = useState<Partial<Record<"display_name" | "timezone", MessageKey>>>({});

  const save = useMutation({
    mutationFn: (patch: { display_name: string; locale: Locale; timezone: string }) => session.api.updateMe(patch),
    onSuccess: (updated) => {
      queryClient.setQueryData(["me"], updated);
      setLocale(updated.locale);
    },
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const v = readForm(form);
    const next: typeof errors = {};
    if (!v.display_name?.trim()) next.display_name = "register.nameRequired";
    if (!v.timezone || !validTimeZone(v.timezone.trim())) next.timezone = "profile.timezoneInvalid";
    setErrors(next);
    const first = (["display_name", "timezone"] as const).find((k) => next[k]);
    if (first) return focusField(form, first);
    save.mutate({ display_name: v.display_name!.trim(), locale: v.locale as Locale, timezone: v.timezone!.trim() });
  }

  return (
    <form onSubmit={onSubmit} noValidate>
      <Field
        label={t("displayName")}
        name="display_name"
        defaultValue={user.display_name}
        autoComplete="name"
        error={errors.display_name && t(errors.display_name)}
      />
      <Field label={t("email")} value={user.email} readOnly />
      <Field label={t("profile.language")}>
        {({ id, describedBy }) => (
          <select id={id} name="locale" defaultValue={user.locale} aria-describedby={describedBy}>
            <option value="th">ไทย</option>
            <option value="en">English</option>
          </select>
        )}
      </Field>
      <Field
        label={t("profile.timezone")}
        name="timezone"
        defaultValue={user.timezone}
        list="timezones"
        hint={t("profile.timezoneHint")}
        error={errors.timezone && t(errors.timezone)}
      />
      <datalist id="timezones">
        {zones.map((z) => (
          <option key={z} value={z} />
        ))}
      </datalist>
      <FormError message={save.isError ? t(errorKey(save.error)) : null} />
      <button type="submit" disabled={save.isPending}>
        {t("save")}
      </button>
      <p role="status">{save.isSuccess ? t("profile.saved") : null}</p>
    </form>
  );
}
