import type { Member, Role } from "@samnak/core";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { FormError } from "../components/FormError";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { canManage, useMe } from "../state/queries";
import { useSession } from "../state/session";
import { RequireWorkspace } from "../components/RequireWorkspace";

const roles: Role[] = ["owner", "admin", "member", "guest"];

export function Members() {
  return (
    <RequireWorkspace>
      {(workspace) => <MemberList workspaceId={workspace.id} myRole={workspace.role} />}
    </RequireWorkspace>
  );
}

function MemberList({ workspaceId, myRole }: { workspaceId: string; myRole: Role }) {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useMe();
  const manage = canManage(myRole);

  const members = useInfiniteQuery({
    queryKey: ["members", workspaceId],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => session.api.listMembers({ cursor: pageParam, limit: 50 }),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["members", workspaceId] });
  const update = useMutation({
    mutationFn: (v: { userId: string; patch: { role?: Role; status?: "active" | "deactivated" } }) =>
      session.api.updateMember(v.userId, v.patch),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (userId: string) => session.api.removeMember(userId),
    onSuccess: refresh,
  });

  const error = members.error ?? update.error ?? remove.error;
  const busy = update.isPending || remove.isPending;

  return (
    <section aria-labelledby="members-heading">
      <h1 id="members-heading">{t("members.title")}</h1>
      {members.isPending && <p role="status">{t("loading")}</p>}
      <FormError message={error ? t(errorKey(error)) : null} />
      {members.data && (
        <table>
          <thead>
            <tr>
              <th scope="col">{t("displayName")}</th>
              <th scope="col">{t("email")}</th>
              <th scope="col">{t("members.role")}</th>
              <th scope="col">{t("members.status")}</th>
              <th scope="col">{t("members.joined")}</th>
              {manage && <th scope="col">{t("members.actions")}</th>}
            </tr>
          </thead>
          <tbody>
            {members.data.pages
              .flatMap((p) => p.items)
              .map((m) => (
                <MemberRow
                  key={m.user_id}
                  member={m}
                  myRole={myRole}
                  manage={manage}
                  busy={busy}
                  joined={formatDateTime(m.joined_at, locale, me.data?.timezone)}
                  onPatch={(patch) => update.mutate({ userId: m.user_id, patch })}
                  onRemove={() => remove.mutate(m.user_id)}
                />
              ))}
          </tbody>
        </table>
      )}
      {members.hasNextPage && (
        <p>
          <button type="button" disabled={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()}>
            {t("loadMore")}
          </button>
        </p>
      )}
    </section>
  );
}

interface RowProps {
  member: Member;
  myRole: Role;
  manage: boolean;
  busy: boolean;
  joined: string;
  onPatch: (patch: { role?: Role; status?: "active" | "deactivated" }) => void;
  onRemove: () => void;
}

function MemberRow({ member, myRole, manage, busy, joined, onPatch, onRemove }: RowProps) {
  const t = useT();
  const [confirming, setConfirming] = useState(false);
  const name = member.display_name || member.email;
  // Only owners grant or remove the owner role, so admins cannot touch an owner.
  const locked = myRole !== "owner" && member.role === "owner";
  const choices = roles.filter((r) => r !== "owner" || myRole === "owner");
  const deactivated = member.status === "deactivated";

  return (
    <tr>
      <th scope="row">{member.display_name}</th>
      <td>{member.email}</td>
      <td>
        {manage ? (
          <select
            aria-label={t("members.roleOf", { name })}
            value={member.role}
            disabled={locked || busy}
            onChange={(e) => onPatch({ role: e.target.value as Role })}
          >
            {choices.concat(choices.includes(member.role) ? [] : [member.role]).map((r) => (
              <option key={r} value={r}>
                {t(`role.${r}`)}
              </option>
            ))}
          </select>
        ) : (
          t(`role.${member.role}`)
        )}
      </td>
      <td>{t(deactivated ? "members.deactivated" : "members.active")}</td>
      <td>{joined}</td>
      {manage && (
        <td className="actions">
          {confirming ? (
            <>
              <button
                type="button"
                className="danger"
                aria-label={t("members.confirmRemove", { name })}
                onClick={() => {
                  setConfirming(false);
                  onRemove();
                }}
              >
                {t("confirm")}
              </button>
              <button type="button" onClick={() => setConfirming(false)}>
                {t("cancel")}
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                disabled={locked || busy}
                aria-label={t(deactivated ? "members.reactivateNamed" : "members.deactivateNamed", { name })}
                onClick={() => onPatch({ status: deactivated ? "active" : "deactivated" })}
              >
                {t(deactivated ? "members.reactivate" : "members.deactivate")}
              </button>
              <button
                type="button"
                disabled={locked || busy}
                aria-label={t("members.removeNamed", { name })}
                onClick={() => setConfirming(true)}
              >
                {t("members.remove")}
              </button>
            </>
          )}
        </td>
      )}
    </tr>
  );
}
