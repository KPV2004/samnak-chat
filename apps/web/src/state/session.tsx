import type { Session, SessionState } from "@samnak/core";
import { createContext, useContext, useSyncExternalStore, type ReactNode } from "react";
import { session as defaultSession } from "../session";

const SessionContext = createContext<Session>(defaultSession);

export function SessionProvider({ session, children }: { session: Session; children: ReactNode }) {
  return <SessionContext value={session}>{children}</SessionContext>;
}

export function useSession(): Session {
  return useContext(SessionContext);
}

export function useSessionState(): SessionState {
  const session = useSession();
  return useSyncExternalStore(session.subscribe, session.getState);
}
