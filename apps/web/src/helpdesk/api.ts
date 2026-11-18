import { createHelpdeskApi } from "@samnak/core";
import { useMemo } from "react";
import { useSession } from "../state/session";

/** The helpdesk calls, on the session's authenticated client. */
export function useHelpdeskApi() {
  const session = useSession();
  return useMemo(() => createHelpdeskApi(session.client), [session]);
}
