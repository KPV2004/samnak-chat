import type { Member, Role } from "@samnak/core";
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { NativeSelect } from "@/components/ui/native-select";
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { AccountLoadError } from "../components/AccountStates";
import { FormError } from "../components/FormError";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import { useCurrentLocale, useT } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { DataTable, ListLoading, LoadMore } from "./module-ui";
import { canManage, useMe } from "../state/queries";
import { useSession } from "../state/session";

const roles: Role[] = ["owner", "admin", "manager", "member", "guest"];

const nameOf = (m: Member) => m.display_name || m.email;

export function Members() {
  const t = useT();
  return (
    <Page width="wide" title={t("members.title")} description={t("members.body")}>
      <RequireWorkspace>
        {(workspace) => <MemberList workspaceId={workspace.id} myRole={workspace.role} />}
      </RequireWorkspace>
    </Page>
  );
}

function MemberList({ workspaceId, myRole }: { workspaceId: string; myRole: Role }) {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const queryClient = useQueryClient();
  const me = useMe();
  const manage = canManage(myRole);
  // The member whose removal is waiting for confirmation in the dialog.
  const [removing, setRemoving] = useState<Member | null>(null);

  const members = useInfiniteQuery({
    queryKey: ["members", workspaceId],
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam }) => session.api.listMembers({ cursor: pageParam, limit: 50 }),
    getNextPageParam: (page) => page.next_cursor || undefined,
  });

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["members", workspaceId] });
  const update = useMutation({
    mutationFn: (v: { userId: string; patch: { role?: Role; status?: "active" | "deactivated" } }) =>
      session.api.updateMember(v.userId, v.patch),
    onSuccess: () => {
      toast.success(t("members.updated"));
      return refresh();
    },
  });
  const remove = useMutation({
    mutationFn: (userId: string) => session.api.removeMember(userId),
    onSuccess: () => {
      toast.success(t("members.removed"));
      return refresh();
    },
  });

  const busy = update.isPending || remove.isPending;

  return (
    <>
      {members.isPending && <ListLoading rows={4} />}
      {members.isError ? (
        <AccountLoadError message={t(errorKey(members.error))} onRetry={() => void members.refetch()} />
      ) : (
        <FormError message={update.error || remove.error ? t(errorKey(update.error ?? remove.error)) : null} />
      )}
      {members.data && (
        <DataTable aria-label={t("members.title")}>
          <TableHeader>
            <TableRow>
              <TableHead scope="col">{t("displayName")}</TableHead>
              <TableHead scope="col">{t("email")}</TableHead>
              <TableHead scope="col">{t("members.role")}</TableHead>
              <TableHead scope="col">{t("members.status")}</TableHead>
              <TableHead scope="col">{t("members.joined")}</TableHead>
              {manage && (
                <TableHead scope="col" className="text-right">
                  {t("members.actions")}
                </TableHead>
              )}
            </TableRow>
          </TableHeader>
          <TableBody>
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
                  onRequestRemove={() => setRemoving(m)}
                />
              ))}
          </TableBody>
        </DataTable>
      )}
      {members.hasNextPage && (
        <LoadMore loading={members.isFetchingNextPage} onClick={() => void members.fetchNextPage()} />
      )}
      <Dialog open={removing !== null} onOpenChange={(open) => !open && setRemoving(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{removing ? t("members.removeTitle", { name: nameOf(removing) }) : null}</DialogTitle>
            <DialogDescription>{t("members.removeBody")}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <DialogClose asChild>
              <Button type="button" variant="outline">
                {t("cancel")}
              </Button>
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              disabled={busy}
              aria-label={removing ? t("members.confirmRemove", { name: nameOf(removing) }) : undefined}
              onClick={() => {
                if (!removing) return;
                setRemoving(null);
                remove.mutate(removing.user_id);
              }}
            >
              {t("confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

interface RowProps {
  member: Member;
  myRole: Role;
  manage: boolean;
  busy: boolean;
  joined: string;
  onPatch: (patch: { role?: Role; status?: "active" | "deactivated" }) => void;
  onRequestRemove: () => void;
}

function MemberRow({ member, myRole, manage, busy, joined, onPatch, onRequestRemove }: RowProps) {
  const t = useT();
  const name = nameOf(member);
  // Only owners grant or remove the owner role, so admins cannot touch an owner.
  const locked = myRole !== "owner" && member.role === "owner";
  const choices = roles.filter((r) => r !== "owner" || myRole === "owner");
  const deactivated = member.status === "deactivated";

  return (
    <TableRow>
      <TableHead scope="row" className="font-medium text-foreground">
        {member.display_name}
      </TableHead>
      <TableCell className="text-muted-foreground">{member.email}</TableCell>
      <TableCell>
        {manage ? (
          <NativeSelect
            size="sm"
            aria-label={t("members.roleOf", { name })}
            containerClassName="min-w-36"
            value={member.role}
            disabled={locked || busy}
            onChange={(e) => onPatch({ role: e.target.value as Role })}
          >
            {choices.concat(choices.includes(member.role) ? [] : [member.role]).map((r) => (
              <option key={r} value={r}>
                {t(`role.${r}`)}
              </option>
            ))}
          </NativeSelect>
        ) : (
          t(`role.${member.role}`)
        )}
      </TableCell>
      <TableCell>
        <Badge variant="outline" className={deactivated ? "text-muted-foreground" : "text-success"}>
          {t(deactivated ? "members.deactivated" : "members.active")}
        </Badge>
      </TableCell>
      <TableCell className="text-muted-foreground">{joined}</TableCell>
      {manage && (
        <TableCell>
          <div className="flex justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={locked || busy}
              aria-label={t(deactivated ? "members.reactivateNamed" : "members.deactivateNamed", { name })}
              onClick={() => onPatch({ status: deactivated ? "active" : "deactivated" })}
            >
              {t(deactivated ? "members.reactivate" : "members.deactivate")}
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="text-destructive hover:text-destructive"
              disabled={locked || busy}
              aria-label={t("members.removeNamed", { name })}
              onClick={onRequestRemove}
            >
              {t("members.remove")}
            </Button>
          </div>
        </TableCell>
      )}
    </TableRow>
  );
}
