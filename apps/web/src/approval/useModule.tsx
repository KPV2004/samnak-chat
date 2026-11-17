import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { Navigate } from "react-router";
import { useT } from "../i18n/useT";
import { useSession } from "../state/session";

/** Whether `/meta` lists a module. False until `/meta` has loaded, so nothing shows before the server says so. */
export function useModuleEnabled(name: string): { enabled: boolean; pending: boolean } {
  const session = useSession();
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => session.client.getMeta() });
  return { enabled: meta.data?.modules.includes(name) ?? false, pending: meta.isPending };
}

/** A page that exists only while its module is enabled; otherwise it leads to the home page. */
export function RequireModule({ name, children }: { name: string; children: ReactNode }) {
  const t = useT();
  const { enabled, pending } = useModuleEnabled(name);
  if (pending) return <p role="status">{t("loading")}</p>;
  if (!enabled) return <Navigate to="/" replace />;
  return <>{children}</>;
}
