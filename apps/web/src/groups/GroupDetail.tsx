import type { Group, GroupMember, GroupMemberInput, Member } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useApprovalApi } from "../approval/api";
import { useMembers } from "../approval/useMembers";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";

interface Props {
  group: Group;
  workspaceId: string;
  canEdit: boolean;
  onDeleted: () => void;
}

/** A group's members, and for owners and admins the controls to rename, delete and staff it (WSP-09). */
export function GroupDetail({ group, workspaceId, canEdit, onDeleted }: Props) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const everyone = useMembers();
  const [confirming, setConfirming] = useState(false);
  const [renameMissing, setRenameMissing] = useState(false);

  const members = useQuery({
    queryKey: ["groups", workspaceId, group.id, "members"],
    queryFn: async () => (await api.listGroupMembers(group.id)).items,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["groups", workspaceId] });
  const rename = useMutation({ mutationFn: (name: string) => api.renameGroup(group.id, name), onSuccess: refresh });
  const remove = useMutation({
    mutationFn: () => api.deleteGroup(group.id),
    onSuccess: () => {
      onDeleted();
      return refresh();
    },
  });
  const save = useMutation({
    mutationFn: (items: GroupMemberInput[]) => api.setGroupMembers(group.id, items),
    onSuccess: refresh,
  });

  function onRename(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = (readForm(form).name ?? "").trim();
    setRenameMissing(name === "");
    if (name === "") return focusField(form, "name");
    rename.mutate(name);
  }

  const error = members.error ?? rename.error ?? remove.error ?? save.error;
  const errorMessage = error ? t(errorKey(error, { 409: "groups.nameTaken" })) : null;

  return (
    <section aria-labelledby="group-heading">
      <h2 id="group-heading">{group.name}</h2>
      {members.isPending && <p role="status">{t("loading")}</p>}

      {canEdit && (
        <>
          <form onSubmit={onRename} noValidate>
            <Field
              label={t("groups.renameLabel")}
              name="name"
              defaultValue={group.name}
              maxLength={100}
              error={renameMissing ? t("groups.nameRequired") : undefined}
            />
            <button type="submit" disabled={rename.isPending}>
              {t("groups.rename")}
            </button>
          </form>
          <p>
            {confirming ? (
              <>
                <button
                  type="button"
                  className="danger"
                  aria-label={t("groups.deleteNamed", { name: group.name })}
                  onClick={() => {
                    setConfirming(false);
                    remove.mutate();
                  }}
                >
                  {t("confirm")}
                </button>{" "}
                <button type="button" onClick={() => setConfirming(false)}>
                  {t("cancel")}
                </button>
              </>
            ) : (
              <>
                <button type="button" className="danger" onClick={() => setConfirming(true)}>
                  {t("groups.delete")}
                </button>{" "}
                <small className="muted">{t("groups.deleteHint")}</small>
              </>
            )}
          </p>
        </>
      )}

      <FormError message={errorMessage} />
      {members.data &&
        (canEdit && everyone.data ? (
          <MemberEditor
            key={group.id}
            initial={members.data}
            candidates={(everyone.data ?? []).filter((m) => m.status === "active")}
            busy={save.isPending}
            saved={save.isSuccess}
            onSave={(items) => save.mutate(items)}
          />
        ) : (
          <ReadOnlyMembers members={members.data} />
        ))}
    </section>
  );
}

function ReadOnlyMembers({ members }: { members: GroupMember[] }) {
  const t = useT();
  if (members.length === 0) return <p>{t("groups.noMembers")}</p>;
  return (
    <ul className="rows" aria-label={t("groups.members")}>
      {members.map((m) => (
        <li key={m.user_id}>
          <span>{m.display_name}</span> {m.is_manager && <span className="badge">{t("groups.manager")}</span>}
        </li>
      ))}
    </ul>
  );
}

type Role = "member" | "manager";

interface EditorProps {
  initial: GroupMember[];
  candidates: Member[];
  busy: boolean;
  saved: boolean;
  onSave: (items: GroupMemberInput[]) => void;
}

/**
 * Picks the members of a group and which of them manage it. Saving sends the whole set. Only
 * active workspace members can be picked, so a deactivated member leaves the group on save.
 */
function MemberEditor({ initial, candidates, busy, saved, onSave }: EditorProps) {
  const t = useT();
  const [roles, setRoles] = useState<Record<string, Role>>(() => {
    const ids = new Set(candidates.map((m) => m.user_id));
    return Object.fromEntries(initial.filter((m) => ids.has(m.user_id)).map((m) => [m.user_id, m.is_manager ? "manager" : "member"]));
  });

  function include(userId: string, on: boolean) {
    setRoles((prev) => {
      const next = { ...prev };
      if (on) next[userId] = "member";
      else delete next[userId];
      return next;
    });
  }

  function makeManager(userId: string, on: boolean) {
    setRoles((prev) => ({ ...prev, [userId]: on ? "manager" : "member" }));
  }

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    onSave(Object.entries(roles).map(([user_id, role]) => ({ user_id, is_manager: role === "manager" })));
  }

  return (
    <form onSubmit={submit}>
      <fieldset>
        <legend>{t("groups.members")}</legend>
        {candidates.map((m) => {
          const name = m.display_name || m.email;
          const role = roles[m.user_id];
          return (
            <div key={m.user_id}>
              <label>
                <input type="checkbox" checked={role !== undefined} onChange={(e) => include(m.user_id, e.target.checked)} /> {name}
              </label>{" "}
              <label>
                <input
                  type="checkbox"
                  aria-label={t("groups.managerOf", { name })}
                  checked={role === "manager"}
                  disabled={role === undefined}
                  onChange={(e) => makeManager(m.user_id, e.target.checked)}
                />{" "}
                {t("groups.manager")}
              </label>
            </div>
          );
        })}
      </fieldset>
      <button type="submit" disabled={busy}>
        {t("groups.save")}
      </button>
      {saved && <p role="status">{t("groups.saved")}</p>}
    </form>
  );
}
