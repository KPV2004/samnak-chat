import type { Group, GroupMember, GroupMemberInput, Member } from "@samnak/core";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Trash2, UserPen } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { useApprovalApi } from "../approval/api";
import { useMembers } from "../approval/useMembers";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Badge } from "@/components/ui/badge";
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

type Dialogs = "rename" | "members" | "delete" | null;

/**
 * A group's members, and for owners and admins the actions to rename, staff and delete it (WSP-09).
 * Each change is a dialog: the members are edited in one, so one save sends the whole set.
 */
export function GroupDetail({ group, workspaceId, canEdit, onDeleted }: Props) {
  const t = useT();
  const api = useApprovalApi();
  const queryClient = useQueryClient();
  const everyone = useMembers();
  const [open, setOpen] = useState<Dialogs>(null);

  const members = useQuery({
    queryKey: ["groups", workspaceId, group.id, "members"],
    queryFn: async () => (await api.listGroupMembers(group.id)).items,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey: ["groups", workspaceId] });
  const rename = useMutation({
    mutationFn: (name: string) => api.renameGroup(group.id, name),
    onSuccess: () => {
      toast.success(t("groups.renamed"));
      return refresh();
    },
  });
  const remove = useMutation({
    mutationFn: () => api.deleteGroup(group.id),
    onSuccess: () => {
      toast.success(t("groups.deleted"));
      onDeleted();
      return refresh();
    },
  });
  const save = useMutation({
    mutationFn: (items: GroupMemberInput[]) => api.setGroupMembers(group.id, items),
    onSuccess: () => {
      toast.success(t("groups.saved"));
      return refresh();
    },
  });

  const listError = members.error ? t(errorKey(members.error)) : null;

  return (
    <Card className="gap-4 py-5">
      <CardHeader className="flex flex-wrap items-start justify-between gap-3">
        <h2 id="group-heading" className="text-base font-semibold">
          {group.name}
        </h2>
        {canEdit && (
          <div className="flex flex-wrap gap-2">
            <Button type="button" variant="outline" size="sm" onClick={() => setOpen("rename")}>
              <Pencil />
              {t("groups.renameOpen")}
            </Button>
            <Button type="button" variant="outline" size="sm" onClick={() => setOpen("members")}>
              <UserPen />
              {t("groups.editMembers")}
            </Button>
            <Button type="button" variant="ghost" size="sm" className="text-destructive hover:text-destructive" onClick={() => setOpen("delete")}>
              <Trash2 />
              {t("groups.delete")}
            </Button>
          </div>
        )}
      </CardHeader>
      <CardContent className="grid gap-3">
        {members.isPending && (
          <div role="status" className="grid gap-2">
            <span className="sr-only">{t("loading")}</span>
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        )}
        <FormError message={listError} />
        {members.data && <ReadOnlyMembers members={members.data} />}
      </CardContent>

      {canEdit && (
        <>
          <Dialog open={open === "rename"} onOpenChange={(o) => setOpen(o ? "rename" : null)}>
            <DialogContent>
              <RenameForm
                defaultName={group.name}
                busy={rename.isPending}
                error={rename.error ? t(errorKey(rename.error, { 409: "groups.nameTaken" })) : null}
                onCancel={() => setOpen(null)}
                onSubmit={(name) => rename.mutate(name, { onSuccess: () => setOpen(null) })}
              />
            </DialogContent>
          </Dialog>
          <Dialog open={open === "members"} onOpenChange={(o) => setOpen(o ? "members" : null)}>
            <DialogContent className="max-h-[90dvh] overflow-y-auto">
              <DialogHeader>
                <DialogTitle>{t("groups.membersOf", { name: group.name })}</DialogTitle>
                <DialogDescription>{t("groups.membersHint")}</DialogDescription>
              </DialogHeader>
              {members.data && everyone.data ? (
                <MemberEditor
                  initial={members.data}
                  candidates={(everyone.data ?? []).filter((m) => m.status === "active")}
                  busy={save.isPending}
                  error={save.error ? t(errorKey(save.error)) : null}
                  onCancel={() => setOpen(null)}
                  onSave={(items) => save.mutate(items, { onSuccess: () => setOpen(null) })}
                />
              ) : (
                <p role="status" className="m-0 text-sm text-muted-foreground">
                  {t("loading")}
                </p>
              )}
            </DialogContent>
          </Dialog>
          <Dialog open={open === "delete"} onOpenChange={(o) => setOpen(o ? "delete" : null)}>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>{t("groups.deleteTitle", { name: group.name })}</DialogTitle>
                <DialogDescription>{t("groups.deleteHint")}</DialogDescription>
              </DialogHeader>
              <FormError message={remove.error ? t(errorKey(remove.error)) : null} />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setOpen(null)}>
                  {t("cancel")}
                </Button>
                <Button
                  type="button"
                  variant="destructive"
                  disabled={remove.isPending}
                  aria-label={t("groups.deleteNamed", { name: group.name })}
                  onClick={() => remove.mutate()}
                >
                  {t("groups.confirm")}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
        </>
      )}
    </Card>
  );
}

function RenameForm({
  defaultName,
  busy,
  error,
  onCancel,
  onSubmit,
}: {
  defaultName: string;
  busy: boolean;
  error: string | null;
  onCancel: () => void;
  onSubmit: (name: string) => void;
}) {
  const t = useT();
  const [missing, setMissing] = useState(false);

  function submit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const form = e.currentTarget;
    const name = (readForm(form).name ?? "").trim();
    setMissing(name === "");
    if (name === "") return focusField(form, "name");
    onSubmit(name);
  }

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t("groups.renameTitle")}</DialogTitle>
        <DialogDescription>{t("groups.renameHint")}</DialogDescription>
      </DialogHeader>
      <form onSubmit={submit} noValidate>
        <Field
          label={t("groups.renameLabel")}
          name="name"
          defaultValue={defaultName}
          maxLength={100}
          error={missing ? t("groups.nameRequired") : undefined}
        />
        <FormError message={error} />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("cancel")}
          </Button>
          <Button type="submit" disabled={busy}>
            {t("groups.rename")}
          </Button>
        </DialogFooter>
      </form>
    </>
  );
}

function ReadOnlyMembers({ members }: { members: GroupMember[] }) {
  const t = useT();
  if (members.length === 0) return <p className="m-0 text-sm text-muted-foreground">{t("groups.noMembers")}</p>;
  return (
    <ul aria-label={t("groups.members")} className="m-0 divide-y rounded-lg border p-0">
      {members.map((m) => (
        <li key={m.user_id} className="flex items-center justify-between gap-2 px-3 py-2 text-sm">
          <span>{m.display_name}</span>
          {m.is_manager && <Badge variant="secondary">{t("groups.manager")}</Badge>}
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
  error: string | null;
  onCancel: () => void;
  onSave: (items: GroupMemberInput[]) => void;
}

/**
 * Picks the members of a group and which of them manage it. Saving sends the whole set. Only
 * active workspace members can be picked, so a deactivated member leaves the group on save.
 */
function MemberEditor({ initial, candidates, busy, error, onCancel, onSave }: EditorProps) {
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
    <form onSubmit={submit} className="grid gap-3">
      <fieldset className="m-0 grid gap-1 rounded-lg border p-2">
        <legend className="px-1 text-sm font-medium">{t("groups.members")}</legend>
        {candidates.map((m) => {
          const name = m.display_name || m.email;
          const id = `member-${m.user_id}`;
          const role = roles[m.user_id];
          return (
            <div key={m.user_id} className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 rounded-md px-2 py-1.5 hover:bg-accent">
              <div className="flex items-center gap-2">
                <Checkbox id={id} checked={role !== undefined} onCheckedChange={(c) => include(m.user_id, c === true)} />
                <Label htmlFor={id} className="leading-normal font-normal">
                  {name}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <Checkbox
                  aria-label={t("groups.managerOf", { name })}
                  checked={role === "manager"}
                  disabled={role === undefined}
                  onCheckedChange={(c) => makeManager(m.user_id, c === true)}
                />
                <span className="text-sm text-muted-foreground">{t("groups.manager")}</span>
              </div>
            </div>
          );
        })}
      </fieldset>
      <FormError message={error} />
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("cancel")}
        </Button>
        <Button type="submit" disabled={busy}>
          {t("groups.save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
