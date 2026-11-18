import type { Locale, User } from "@samnak/core";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountLoadError, AccountLoading } from "../components/AccountStates";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { translate } from "../i18n/i18n";
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
    <Page width="narrow" title={t("profile.title")} description={t("profile.body")}>
      {me.isPending && (
        <AccountLoading>
          <Card>
            <CardContent className="grid gap-4">
              {[0, 1, 2, 3].map((i) => (
                <div key={i} className="grid gap-1.5">
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="h-9 w-full" />
                </div>
              ))}
            </CardContent>
          </Card>
        </AccountLoading>
      )}
      {me.isError && <AccountLoadError message={t(errorKey(me.error))} onRetry={() => void me.refetch()} />}
      {me.data && <ProfileForm user={me.data} />}
    </Page>
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
      // The page switches language with the save, so the confirmation is in the new language.
      toast.success(translate(updated.locale, "profile.saved"));
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
      <Card>
        <CardContent className="grid gap-4">
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
              <NativeSelect
                id={id}
                name="locale"
                defaultValue={user.locale}
                aria-describedby={describedBy}
                containerClassName="w-full"
              >
                <option value="th">ไทย</option>
                <option value="en">English</option>
              </NativeSelect>
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
        </CardContent>
        <CardFooter className="justify-end">
          <Button type="submit" disabled={save.isPending}>
            {save.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
            {t("save")}
          </Button>
        </CardFooter>
      </Card>
    </form>
  );
}
