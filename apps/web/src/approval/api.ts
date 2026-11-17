import { createApprovalApi } from "@samnak/core";
import { useMemo } from "react";
import { useSession } from "../state/session";

/** The approval and groups calls, on the session's authenticated client. */
export function useApprovalApi() {
  const session = useSession();
  return useMemo(() => createApprovalApi(session.client), [session]);
}
