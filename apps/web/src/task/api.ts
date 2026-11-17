import { createExtensionsApi, createTaskApi, type ChatCommand, type ChatMessageAction, type ExtensionsApi, type TaskApi } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";
import { useSession } from "../state/session";
import { useEnabledModules } from "./modules";

export function useTaskApi(): TaskApi {
  const session = useSession();
  return useMemo(() => createTaskApi(session.client), [session]);
}

export function useExtensionsApi(): ExtensionsApi {
  const session = useSession();
  return useMemo(() => createExtensionsApi(session.client), [session]);
}

/**
 * The slash commands and message actions the server registers (CHAT-16). Those
 * of a module `/meta` does not list are dropped, so a disabled module adds no menu entry.
 */
export function useChatExtensions(): { commands: ChatCommand[]; messageActions: ChatMessageAction[] } {
  const api = useExtensionsApi();
  const enabled = useEnabledModules();
  const { data } = useQuery({
    queryKey: ["chat-extensions"],
    staleTime: 60_000,
    queryFn: () => api.listExtensions(),
  });
  return useMemo(
    () => ({
      commands: (data?.commands ?? []).filter((c) => enabled.includes(c.module)),
      messageActions: (data?.message_actions ?? []).filter((a) => enabled.includes(a.module)),
    }),
    [data, enabled],
  );
}
