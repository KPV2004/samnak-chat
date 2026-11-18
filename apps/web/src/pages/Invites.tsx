import type { Invite, InviteRole } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2Icon, Lock, UserPlus } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardFooter } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { NativeSelect } from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { AccountLoadError, AccountLoading } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { emailPattern, readForm } from "../lib/form";
import { formatDateTime } from "../lib/format";
import { canManage, useMe } from "../state/queries";
import { useSession } from "../state/session";

const inviteRoles: InviteRole[] = ["admin", "member", "guest"];

export function Invites() {
  const t = useT();
  return (
    <Page title={t("invites.title")}>
      <RequireWorkspace>
        {(workspace) =>
          canManage(workspace.role) ? (
            <InviteManager workspaceId={workspace.id} />
          ) : (
            <EmptyState icon={Lock} title={t("invites.adminOnly")} />
          )
        }
      </RequireWorkspace>
    </Page>
  );
}

function InviteManager({ workspaceId }: { workspaceId: string }) {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useMe();
  const [emailError, setEmailError] = useState(false);
  const [created, setCreated] = useState<Invite | null>(null);

  const invites = useQuery({
    queryKey: ["invites", workspaceId],
    queryFn: async () => (await session.api.listInvites()).items,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["invites", workspaceId] });
  const create = useMutation({
    mutationFn: (v: { email?: string; role: InviteRole }) => session.api.createInvite(v),
    onSuccess: (invite) => {
      setCreated(invite);
      return refresh();
    },
  });
  const revoke = useMutation({
    mutationFn: (id: string) => session.api.revokeInvite(id),
    onSuccess: () => {
      toast.success(t("invites.revoked"));
      return refresh();
    },
  });

  function onSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const v = readForm(form);
    const email = v.email?.trim();
    const invalid = Boolean(email) && !emailPattern.test(email!);
    setEmailError(invalid);
    if (invalid) return (form.elements.namedItem("email") as HTMLElement).focus();
    create.mutate({ ...(email ? { email } : {}), role: v.role as InviteRole });
  }

  const link = created?.token ? `${window.location.origin}/invite/${created.token}` : null;

  async function copy() {
    if (!link) return;
    try {
      await navigator.clipboard.writeText(link);
      toast.success(t("invites.copied"));
    } catch {
      // clipboard blocked: the link stays selectable in the field
    }
  }

  const actionError = create.error ?? revoke.error;

  return (
    <div className="grid gap-6">
      <form onSubmit={onSubmit} noValidate>
        <Card>
          <CardContent className="grid gap-4 md:grid-cols-2 md:items-start">
            <Field
              label={t("invites.email")}
              name="email"
              type="email"
              hint={t("invites.emailHint")}
              error={emailError ? t("invites.emailInvalid") : undefined}
            />
            <Field label={t("members.role")}>
              {({ id, describedBy }) => (
                <NativeSelect id={id} name="role" defaultValue="member" aria-describedby={describedBy} containerClassName="w-full">
                  {inviteRoles.map((r) => (
                    <option key={r} value={r}>
                      {t(`role.${r}`)}
                    </option>
                  ))}
                </NativeSelect>
              )}
            </Field>
          </CardContent>
          <CardFooter className="justify-end">
            <Button type="submit" disabled={create.isPending}>
              {create.isPending && <Loader2Icon className="animate-spin" aria-hidden="true" />}
              {t("invites.create")}
            </Button>
          </CardFooter>
        </Card>
      </form>

      {link && (
        <Card>
          <CardContent>
            <Field label={t("invites.link")} hint={t("invites.linkOnce")}>
              {({ id, describedBy }) => (
                <div className="flex gap-2">
                  <Input
                    id={id}
                    aria-describedby={describedBy}
                    value={link}
                    readOnly
                    onFocus={(e) => e.currentTarget.select()}
                  />
                  <Button type="button" variant="outline" onClick={() => void copy()}>
                    {t("invites.copy")}
                  </Button>
                </div>
              )}
            </Field>
          </CardContent>
        </Card>
      )}

      {invites.isPending && (
        <AccountLoading>
          <div className="divide-y overflow-hidden rounded-lg border">
            {[0, 1].map((i) => (
              <div key={i} className="flex items-center gap-3 px-4 py-3">
                <div className="grid flex-1 gap-2">
                  <Skeleton className="h-4 w-48" />
                  <Skeleton className="h-3 w-32" />
                </div>
              </div>
            ))}
          </div>
        </AccountLoading>
      )}
      {invites.isError ? (
        <AccountLoadError message={t(errorKey(invites.error))} onRetry={() => void invites.refetch()} />
      ) : (
        <FormError message={actionError ? t(errorKey(actionError)) : null} />
      )}
      {invites.data &&
        (invites.data.length === 0 ? (
          <EmptyState icon={UserPlus} title={t("invites.none")} description={t("invites.emptyHint")} />
        ) : (
          <section aria-labelledby="pending-invites-heading" className="grid gap-3">
            <h2 id="pending-invites-heading" className="text-base font-semibold">
              {t("invites.pending")}
            </h2>
            <ul className="divide-y overflow-hidden rounded-lg border" aria-labelledby="pending-invites-heading">
              {invites.data.map((i) => {
                const who = i.email ?? t("invites.anyone");
                return (
                  <li key={i.id} className="flex items-center gap-3 px-4 py-3">
                    <div className="grid min-w-0 flex-1 gap-0.5">
                      <p className="m-0 flex flex-wrap items-center gap-2 text-sm">
                        <span className="font-medium break-all">{who}</span>
                        <Badge variant="outline">{t(`role.${i.role}`)}</Badge>
                      </p>
                      <p className="m-0 text-xs text-muted-foreground">
                        {t("invites.expires", { time: formatDateTime(i.expires_at, locale, me.data?.timezone) })}
                      </p>
                    </div>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={revoke.isPending}
                      aria-label={t("invites.revokeFor", { who })}
                      onClick={() => revoke.mutate(i.id)}
                    >
                      {t("invites.revoke")}
                    </Button>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
    </div>
  );
}
