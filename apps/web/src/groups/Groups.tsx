import type { Group } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus, UsersRound } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useApprovalApi } from "../approval/api";
import { useNameOf } from "../approval/useMembers";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { Field } from "../components/Field";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { focusField, readForm } from "../lib/form";
import { DataList, DataRow, ListLoading, stretchedLink } from "../pages/module-ui";
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
  const nameOf = useNameOf();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);

  const groups = useQuery({
    queryKey: ["groups", workspaceId, "list"],
    queryFn: async () => (await api.listGroups()).items,
  });

  const selected: Group | undefined = groups.data?.find((g) => g.id === selectedId);
  const hasGroups = (groups.data?.length ?? 0) > 0;

  return (
    <Page
      title={t("groups.title")}
      width="wide"
      description={
        <>
          <span className="block">{t("groups.intro")}</span>
          {!canEdit && <span className="block">{t("groups.editHint")}</span>}
        </>
      }
      actions={
        canEdit && (
          <Button type="button" onClick={() => setCreating(true)}>
            <Plus />
            {t("groups.new")}
          </Button>
        )
      }
    >
      <div className="grid items-start gap-6 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
        <div className="grid gap-3">
          {groups.isPending && <ListLoading rows={2} />}
          {groups.isError && (
            <AccountLoadError message={t(errorKey(groups.error))} onRetry={() => void groups.refetch()} />
          )}
          {groups.data && !hasGroups && (
            <EmptyState
              icon={UsersRound}
              title={t("groups.none")}
              description={canEdit ? t("groups.noGroupsYet") : undefined}
            />
          )}
          {hasGroups && (
            <DataList aria-label={t("groups.title")}>
              {(groups.data ?? []).map((g) => (
                <DataRow key={g.id} interactive selected={g.id === selectedId} className="flex-col items-stretch">
                  <button
                    type="button"
                    aria-pressed={g.id === selectedId}
                    onClick={() => setSelectedId(g.id)}
                    className={cn("w-fit text-left font-medium", stretchedLink)}
                  >
                    {g.name}
                  </button>
                  <p className="m-0 text-xs text-muted-foreground">
                    <span>{t("groups.memberCount", { n: g.member_count })}</span>
                    {" · "}
                    <span>
                      {g.manager_ids.length > 0
                        ? t("groups.managers", { names: g.manager_ids.map((id) => nameOf(id)).join(", ") })
                        : t("groups.noManager")}
                    </span>
                  </p>
                </DataRow>
              ))}
            </DataList>
          )}
        </div>

        {selected ? (
          <GroupDetail
            key={selected.id}
            group={selected}
            workspaceId={workspaceId}
            canEdit={canEdit}
            onDeleted={() => setSelectedId(null)}
          />
        ) : (
          hasGroups && <EmptyState icon={UsersRound} title={t("groups.select")} />
        )}
      </div>

      {canEdit && <CreateGroupDialog open={creating} onOpenChange={setCreating} workspaceId={workspaceId} />}
    </Page>
  );
}

/** The dialog that creates a group. It stays open on a refused name, so the owner can change it. */
function CreateGroupDialog({
  open,
  onOpenChange,
  workspaceId,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  workspaceId: string;
}) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const [nameMissing, setNameMissing] = useState(false);
  const create = useMutation({
    mutationFn: (name: string) => api.createGroup(name),
    onSuccess: () => {
      toast.success(t("groups.created"));
      return queryClient.invalidateQueries({ queryKey: ["groups", workspaceId] });
    },
  });

  function onCreate(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = (readForm(form).name ?? "").trim();
    setNameMissing(name === "");
    if (name === "") return focusField(form, "name");
    create.mutate(name, {
      onSuccess: () => {
        form.reset();
        setNameMissing(false);
        onOpenChange(false);
      },
    });
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        onOpenChange(next);
        if (!next) {
          create.reset();
          setNameMissing(false);
        }
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("groups.new")}</DialogTitle>
          <DialogDescription>{t("groups.nameHint")}</DialogDescription>
        </DialogHeader>
        <form onSubmit={onCreate} noValidate>
          <Field
            label={t("groups.name")}
            name="name"
            maxLength={100}
            error={nameMissing ? t("groups.nameRequired") : undefined}
          />
          <FormError message={create.error ? t(errorKey(create.error, { 409: "groups.nameTaken" })) : null} />
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={create.isPending}>
              {t("groups.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
