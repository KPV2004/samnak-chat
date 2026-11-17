import type { Group } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { useApprovalApi } from "../approval/api";
import { useNameOf } from "../approval/useMembers";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";
import { canManage } from "../state/queries";
import { GroupDetail } from "./GroupDetail";

/**
 * The Groups page (WSP-09). Everyone sees the groups and their managers; owners and admins
 * create, rename, delete and staff them, as the server allows (APR-02 defaults to a group's manager).
 */
export function Groups() {
  return (
    <RequireWorkspace>
      {(workspace) => <GroupsPage workspaceId={workspace.id} canEdit={canManage(workspace.role)} />}
    </RequireWorkspace>
  );
}

function GroupsPage({ workspaceId, canEdit }: { workspaceId: string; canEdit: boolean }) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const nameOf = useNameOf();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [nameMissing, setNameMissing] = useState(false);

  const groups = useQuery({
    queryKey: ["groups", workspaceId, "list"],
    queryFn: async () => (await api.listGroups()).items,
  });
  const create = useMutation({
    mutationFn: (name: string) => api.createGroup(name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["groups", workspaceId] }),
  });

  function onCreate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = (readForm(form).name ?? "").trim();
    setNameMissing(name === "");
    if (name === "") return focusField(form, "name");
    create.mutate(name, { onSuccess: () => form.reset() });
  }

  const selected: Group | undefined = groups.data?.find((g) => g.id === selectedId);

  return (
    <section aria-labelledby="groups-heading">
      <h1 id="groups-heading">{t("groups.title")}</h1>
      <p>{t("groups.intro")}</p>
      {!canEdit && <p className="muted">{t("groups.editHint")}</p>}

      {canEdit && (
        <section aria-labelledby="create-group-heading">
          <h2 id="create-group-heading">{t("groups.create")}</h2>
          <form onSubmit={onCreate} noValidate>
            <Field
              label={t("groups.name")}
              name="name"
              maxLength={100}
              hint={t("groups.nameHint")}
              error={nameMissing ? t("groups.nameRequired") : undefined}
            />
            <button type="submit" disabled={create.isPending}>
              {t("groups.create")}
            </button>
            <FormError message={create.error ? t(errorKey(create.error, { 409: "groups.nameTaken" })) : null} />
          </form>
        </section>
      )}

      {groups.isPending && <p role="status">{t("loading")}</p>}
      {groups.isError && <FormError message={t(errorKey(groups.error))} />}
      {groups.data && groups.data.length === 0 && <p>{t("groups.none")}</p>}
      {groups.data && groups.data.length > 0 && (
        <ul className="rows" aria-label={t("groups.title")}>
          {groups.data.map((g) => (
            <li key={g.id}>
              <div>
                <button type="button" aria-pressed={g.id === selectedId} onClick={() => setSelectedId(g.id)}>
                  {g.name}
                </button>{" "}
                <small className="muted">{t("groups.memberCount", { n: g.member_count })}</small>
                <br />
                <small className="muted">
                  {g.manager_ids.length > 0
                    ? t("groups.managers", { names: g.manager_ids.map((id) => nameOf(id)).join(", ") })
                    : t("groups.noManager")}
                </small>
              </div>
            </li>
          ))}
        </ul>
      )}

      {selected ? (
        <GroupDetail
          key={selected.id}
          group={selected}
          workspaceId={workspaceId}
          canEdit={canEdit}
          onDeleted={() => setSelectedId(null)}
        />
      ) : (
        groups.data && groups.data.length > 0 && <p>{t("groups.select")}</p>
      )}
    </section>
  );
}
