import { ApiError, createApiClient, type ApiClient, type FetchLike } from "../api/client";
import { createM1Api, type LoginInput, type M1Api, type RegisterInput, type TokenPair, type User } from "../api/m1";

/** What a client keeps between launches. */
export interface SessionData {
  accessToken: string;
  refreshToken: string;
  deviceId: string;
  workspaceId: string | null;
}

/** Where the session is persisted. Each platform brings its own (web: localStorage, mobile: secure store). */
export interface TokenStore {
  load(): SessionData | null;
  save(data: SessionData | null): void;
}

export class MemoryTokenStore implements TokenStore {
  constructor(private data: SessionData | null = null) {}
  load() {
    return this.data;
  }
  save(data: SessionData | null) {
    this.data = data;
  }
}

/** Immutable snapshot, replaced on every change so it works with `useSyncExternalStore`. */
export interface SessionState {
  signedIn: boolean;
  deviceId: string | null;
  workspaceId: string | null;
}

export interface SessionOptions {
  baseUrl: string;
  store: TokenStore;
  fetch?: FetchLike;
}

export interface Session {
  /** Authenticated client: adds the headers and refreshes the token once on 401. */
  readonly client: ApiClient;
  readonly api: M1Api;
  getState(): SessionState;
  subscribe(listener: () => void): () => void;
  register(input: RegisterInput): Promise<User>;
  login(input: LoginInput): Promise<TokenPair>;
  /** Signs out on the server (best effort) and always clears local state. */
  logout(allDevices: boolean): Promise<void>;
  setWorkspaceId(id: string | null): void;
}

const signedOut: SessionState = { signedIn: false, deviceId: null, workspaceId: null };

function toState(d: SessionData | null): SessionState {
  return d ? { signedIn: true, deviceId: d.deviceId, workspaceId: d.workspaceId } : signedOut;
}

export function createSession(opts: SessionOptions): Session {
  let data = opts.store.load();
  let state = toState(data);
  const listeners = new Set<() => void>();

  function commit(next: SessionData | null) {
    data = next;
    opts.store.save(next);
    state = toState(next);
    listeners.forEach((l) => l());
  }

  function accept(pair: TokenPair, workspaceId: string | null) {
    commit({
      accessToken: pair.access_token,
      refreshToken: pair.refresh_token,
      deviceId: pair.device_id,
      workspaceId,
    });
  }

  let inflight: Promise<void> | null = null;

  // Resolves true when the caller should retry with the current token.
  async function refresh(rejectedToken: string): Promise<boolean> {
    const current = data;
    if (!current) return false;
    // Another request already replaced the token this one was sent with.
    if (current.accessToken !== rejectedToken) return true;
    inflight ??= (async () => {
      try {
        accept(await api.refresh(current.refreshToken), current.workspaceId);
      } catch (err) {
        // Only a server answer ends the session; a dropped connection may recover.
        if (err instanceof ApiError) commit(null);
        throw err;
      } finally {
        inflight = null;
      }
    })();
    await inflight;
    return true;
  }

  const client = createApiClient({
    baseUrl: opts.baseUrl,
    fetch: opts.fetch,
    getToken: () => data?.accessToken,
    getWorkspaceId: () => data?.workspaceId,
    onUnauthorized: refresh,
  });
  const api = createM1Api(client);

  return {
    client,
    api,
    getState: () => state,
    subscribe(listener) {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    register: (input) => api.register(input),
    async login(input) {
      const pair = await api.login(input);
      accept(pair, null);
      return pair;
    },
    async logout(allDevices) {
      try {
        await api.logout(allDevices);
      } catch {
        // The server may be unreachable or the token already dead; sign out locally regardless.
      }
      commit(null);
    },
    setWorkspaceId(id) {
      if (!data || data.workspaceId === id) return;
      commit({ ...data, workspaceId: id });
    },
  };
}
