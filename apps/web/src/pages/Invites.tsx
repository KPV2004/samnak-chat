import type { Invite, InviteRole } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
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
    <RequireWorkspace>
      {(workspace) => (
        <section aria-labelledby="invites-heading">
          <h1 id="invites-heading">{t("invites.title")}</h1>
          {canManage(workspace.role) ? (
            <InviteManager workspaceId={workspace.id} />
          ) : (
            <p>{t("invites.adminOnly")}</p>
          )}
        </section>
      )}
    </RequireWorkspace>
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
  const [copied, setCopied] = useState(false);

  const invites = useQuery({
    queryKey: ["invites", workspaceId],
    queryFn: async () => (await session.api.listInvites()).items,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["invites", workspaceId] });
  const create = useMutation({
    mutationFn: (v: { email?: string; role: InviteRole }) => session.api.createInvite(v),
    onSuccess: (invite) => {
      setCreated(invite);
      setCopied(false);
      return refresh();
    },
  });
  const revoke = useMutation({ mutationFn: (id: string) => session.api.revokeInvite(id), onSuccess: refresh });

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
      setCopied(true);
    } catch {
      // clipboard blocked: the link stays selectable in the field
    }
  }

  const error = invites.error ?? create.error ?? revoke.error;

  return (
    <>
      <form onSubmit={onSubmit} noValidate>
        <Field
          label={t("invites.email")}
          name="email"
          type="email"
          hint={t("invites.emailHint")}
          error={emailError ? t("login.emailRequired") : undefined}
        />
        <Field label={t("members.role")}>
          {({ id, describedBy }) => (
            <select id={id} name="role" defaultValue="member" aria-describedby={describedBy}>
              {inviteRoles.map((r) => (
                <option key={r} value={r}>
                  {t(`role.${r}`)}
                </option>
              ))}
            </select>
          )}
        </Field>
        <button type="submit" disabled={create.isPending}>
          {t("invites.create")}
        </button>
      </form>

      {link && (
        <div className="callout">
          <Field label={t("invites.link")} value={link} readOnly onFocus={(e) => e.currentTarget.select()} hint={t("invites.linkOnce")} />
          <button type="button" onClick={() => void copy()}>
            {t("invites.copy")}
          </button>
          <span role="status">{copied ? t("invites.copied") : null}</span>
        </div>
      )}

      <FormError message={error ? t(errorKey(error)) : null} />
      {invites.isPending && <p role="status">{t("loading")}</p>}
      {invites.data &&
        (invites.data.length === 0 ? (
          <p>{t("invites.none")}</p>
        ) : (
          <ul className="rows" aria-label={t("invites.pending")}>
            {invites.data.map((i) => {
              const who = i.email ?? t("invites.anyone");
              return (
                <li key={i.id}>
                  <div>
                    <strong>{who}</strong> <span className="badge">{t(`role.${i.role}`)}</span>
                    <br />
                    <small className="muted">
                      {t("invites.expires", { time: formatDateTime(i.expires_at, locale, me.data?.timezone) })}
                    </small>
                  </div>
                  <button
                    type="button"
                    disabled={revoke.isPending}
                    aria-label={t("invites.revokeFor", { who })}
                    onClick={() => revoke.mutate(i.id)}
                  >
                    {t("invites.revoke")}
                  </button>
                </li>
              );
            })}
          </ul>
        ))}
    </>
  );
}
