import type { Role, User, Workspace } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useEffect } from "react";
import { useSession, useSessionState } from "./session";

export function useMe() {
  const session = useSession();
  const { signedIn } = useSessionState();
  return useQuery<User>({ queryKey: ["me"], queryFn: () => session.api.getMe(), enabled: signedIn });
}

export function useWorkspaces() {
  const session = useSession();
  const { signedIn, workspaceId } = useSessionState();
  const query = useQuery<Workspace[]>({
    queryKey: ["workspaces"],
    queryFn: async () => (await session.api.listWorkspaces()).items,
    enabled: signedIn,
  });
  const current = query.data?.find((w) => w.id === workspaceId) ?? null;
  return { query, workspaces: query.data, current };
}

/** Keeps the selected workspace valid: falls back to the first one when it is missing or no longer mine. */
export function useSyncWorkspace() {
  const session = useSession();
  const { workspaces, current } = useWorkspaces();
  useEffect(() => {
    if (workspaces && !current) session.setWorkspaceId(workspaces[0]?.id ?? null);
  }, [workspaces, current, session]);
}

export function canManage(role: Role | undefined): boolean {
  return role === "owner" || role === "admin";
}
