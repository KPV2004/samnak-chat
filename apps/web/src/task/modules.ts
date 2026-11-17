import { useQuery } from "@tanstack/react-query";
import { useSession } from "../state/session";

const noModules: string[] = [];

/** `/meta`, the same query the notification bell makes, so the cache is shared. */
export function useMeta() {
  const session = useSession();
  return useQuery({ queryKey: ["meta"], queryFn: () => session.client.getMeta() });
}

/** The modules `/meta` lists; empty until it is known, so nothing module-specific shows early. */
export function useEnabledModules(): string[] {
  return useMeta().data?.modules ?? noModules;
}

/** Whether `/meta` lists the module. A module that is off leaves no trace in the UI. */
export function useModuleEnabled(name: string): boolean {
  return useEnabledModules().includes(name);
}
