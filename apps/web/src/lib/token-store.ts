import type { SessionData, TokenStore } from "@samnak/core";

const KEY = "samnak.session";

function isSessionData(v: unknown): v is SessionData {
  const d = v as Partial<SessionData> | null;
  return (
    typeof d?.accessToken === "string" &&
    typeof d.refreshToken === "string" &&
    typeof d.deviceId === "string" &&
    (typeof d.workspaceId === "string" || d.workspaceId === null)
  );
}

/** Keeps the session in `localStorage`. Unreadable or malformed data counts as signed out. */
export function createLocalStorageTokenStore(storage: Pick<Storage, "getItem" | "setItem" | "removeItem">): TokenStore {
  return {
    load() {
      try {
        const raw = storage.getItem(KEY);
        const parsed: unknown = raw ? JSON.parse(raw) : null;
        return isSessionData(parsed) ? parsed : null;
      } catch {
        return null;
      }
    },
    save(data) {
      try {
        if (data) storage.setItem(KEY, JSON.stringify(data));
        else storage.removeItem(KEY);
      } catch {
        // storage full or blocked: the session lasts until reload
      }
    },
  };
}
