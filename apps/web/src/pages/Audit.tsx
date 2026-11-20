import type { AuditAction, AuditEntry, Role } from "@samnak/core";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { useMemo } from "react";
import { TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useMembers } from "../approval/useMembers";
import { AccountLoadError } from "../components/AccountStates";
import { EmptyState } from "../components/EmptyState";
import { Page } from "../components/Page";
import { RequireWorkspace } from "../components/RequireWorkspace";
import type { MessageKey } from "../i18n/messages";
import { useCurrentLocale, useT, type T } from "../i18n/useT";
import { errorKey } from "../lib/errors";
import { formatDateTime } from "../lib/format";
import { canManage, useMe } from "../state/queries";
import { useSession } from "../state/session";
import { DataTable, ListLoading, LoadMore } from "./module-ui";

const actionKey: Record<AuditAction, MessageKey> = {
  "invite.created": "audit.action.invite.created",
  "invite.revoked": "audit.action.invite.revoked",
  "member.updated": "audit.action.member.updated",
  "member.removed": "audit.action.member.removed",
  "group.created": "audit.action.group.created",
  "group.updated": "audit.action.group.updated",
  "group.deleted": "audit.action.group.deleted",
  "group.members_set": "audit.action.group.members_set",
};

export function Audit() {
  const t = useT();
  return (
    <Page width="wide" title={t("audit.title")} description={t("audit.body")}>
      <RequireWorkspace>
        {(workspace) =>
          canManage(workspace.role) ? (
            <AuditList workspaceId={workspace.id} />
          ) : (
            <EmptyState icon={Lock} title={t("audit.adminOnly")} />
          )
        }
      </RequireWorkspace>
    </Page>
  );
}

function AuditList({ workspaceId }: { workspaceId: string }) {
  const t = useT();
  const locale = useCurrentLocale();
  const session = useSession();
  const me = useMe();
  const audit = useInfiniteQuery({
    queryKey: ["audit", workspaceId],
    initialPageParam: "",
    queryFn: ({ pageParam }) => session.api.listAudit({ cursor: pageParam, limit: 50 }),
    getNextPageParam: (page) => page.next_cursor || undefined,
  });
  const members = useMembers();
  // Names come from the members list; someone no longer in it (a removed member) has no name to show.
  const names = useMemo(
    () => new Map((members.data ?? []).map((m) => [m.user_id, m.display_name || m.email])),
    [members.data],
  );
  const nameOf = (userId: string) => names.get(userId) ?? t("audit.formerMember");

  if (audit.isPending || members.isPending) return <ListLoading rows={4} />;
  if (audit.isError || members.isError) {
    const error = audit.error ?? members.error;
    return (
      <AccountLoadError
        message={t(errorKey(error))}
        onRetry={() => {
          void audit.refetch();
          void members.refetch();
        }}
      />
    );
  }

  const entries = audit.data.pages.flatMap((p) => p.items);
  if (entries.length === 0) return <EmptyState title={t("audit.empty")} />;

  return (
    <>
      <DataTable aria-label={t("audit.title")}>
        <TableHeader>
          <TableRow>
            <TableHead scope="col">{t("audit.when")}</TableHead>
            <TableHead scope="col">{t("audit.actor")}</TableHead>
            <TableHead scope="col">{t("audit.action")}</TableHead>
            <TableHead scope="col">{t("audit.target")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {entries.map((entry) => (
            <TableRow key={entry.id}>
              <TableCell className="whitespace-nowrap text-muted-foreground">
                <time dateTime={entry.created_at}>{formatDateTime(entry.created_at, locale, me.data?.timezone)}</time>
              </TableCell>
              <TableCell>{nameOf(entry.actor_id)}</TableCell>
              <TableCell>
                <div className="grid gap-0.5">
                  <span>{t(actionKey[entry.action])}</span>
                  {describeChanges(entry, t).map((line) => (
                    <span key={line} className="text-sm text-muted-foreground">
                      {line}
                    </span>
                  ))}
                </div>
              </TableCell>
              <TableCell className="text-muted-foreground">{targetOf(entry, nameOf, t)}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </DataTable>
      {audit.hasNextPage && <LoadMore loading={audit.isFetchingNextPage} onClick={() => void audit.fetchNextPage()} />}
    </>
  );
}

interface Change<V> {
  from: V;
  to: V;
}

/** `data[key]` when it records a from and a to, as `member.updated` does for a role or status it changed. */
function changeOf<V>(entry: AuditEntry, key: string): Change<V> | null {
  const value = entry.data[key];
  return value && typeof value === "object" && "from" in value && "to" in value ? (value as Change<V>) : null;
}

function describeChanges(entry: AuditEntry, t: T): string[] {
  const role = changeOf<Role>(entry, "role");
  const active = changeOf<boolean>(entry, "active");
  const lines: string[] = [];
  if (role) lines.push(t("audit.change.role", { from: t(`role.${role.from}`), to: t(`role.${role.to}`) }));
  if (active) {
    const status = (on: boolean) => t(on ? "members.active" : "members.deactivated");
    lines.push(t("audit.change.active", { from: status(active.from), to: status(active.to) }));
  }
  return lines;
}

function targetOf(entry: AuditEntry, nameOf: (userId: string) => string, t: T): string {
  switch (entry.target_type) {
    case "member":
      return nameOf(entry.target_id);
    case "invite":
      return t("audit.target.invite");
    case "group":
      return t("audit.target.group");
  }
}
