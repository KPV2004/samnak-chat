import { serverEventTypes, type ClientEventMap, type ServerEventMap, type ServerEventType } from "./events";

/**
 * The part of a WebSocket this client uses. The browser's `WebSocket` fits, so
 * the web app passes `(url) => new WebSocket(url)` and this package needs no
 * DOM types.
 */
export interface SocketLike {
  send(data: string): void;
  close(code?: number, reason?: string): void;
  // `never` parameters accept the DOM's event-taking handlers; the client
  // assigns handlers typed with the fields it reads (see `SocketEvents`).
  onopen: ((ev: never) => void) | null;
  onmessage: ((ev: never) => void) | null;
  onclose: ((ev: never) => void) | null;
  onerror: ((ev: never) => void) | null;
}

/** The event fields the client reads. */
interface SocketEvents {
  message: { data: unknown };
  close: { code: number };
}

const asHandler = <E>(fn: (ev: E) => void) => fn as unknown as (ev: never) => void;

export type SocketFactory = (url: string) => SocketLike;

export type RealtimeStatus = "idle" | "connecting" | "authenticating" | "ready" | "reconnecting" | "closed";

export interface RealtimeOptions {
  /** Full URL, e.g. `wss://host/api/v1/ws`. */
  url: string;
  createSocket: SocketFactory;
  /** Read at every connection attempt, so a refreshed token is picked up. */
  getToken: () => string | null | undefined;
  /**
   * Called when the hub closes with 4401 (token rejected), with the token that was used.
   * Resolve `true` once a fresh token is available (e.g. after a session refresh).
   */
  onUnauthorized?: (rejectedToken: string) => Promise<boolean>;
  pingIntervalMs?: number;
  handshakeTimeoutMs?: number;
  minBackoffMs?: number;
  maxBackoffMs?: number;
  /** Returns [0, 1); replaceable in tests. */
  random?: () => number;
}

export interface RealtimeClient {
  /** Starts connecting and keeps the connection up until `close()`. */
  connect(): void;
  close(): void;
  /**
   * Call after the access token was refreshed. The hub authenticates only in
   * the first frame, so this reconnects at once with the current token.
   */
  reauth(): void;
  on<T extends ServerEventType>(type: T, handler: (data: ServerEventMap[T]) => void): () => void;
  /** Sends an envelope; false when the connection is not ready. */
  send<T extends keyof ClientEventMap>(type: T, data: ClientEventMap[T]): boolean;
  getStatus(): RealtimeStatus;
  /** For `useSyncExternalStore`, together with `getStatus`. */
  subscribe(listener: () => void): () => void;
}

const closeUnauthorized = 4401;

// Timer functions are not in this package's lib typings; resolve them per call
// so fake timers installed later are honoured.
interface Timers {
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(handle: unknown): void;
  setInterval(fn: () => void, ms: number): unknown;
  clearInterval(handle: unknown): void;
}
const timers = (): Timers => globalThis as unknown as Timers;

export function createRealtimeClient(opts: RealtimeOptions): RealtimeClient {
  const pingMs = opts.pingIntervalMs ?? 30_000;
  const handshakeMs = opts.handshakeTimeoutMs ?? 10_000;
  const minBackoff = opts.minBackoffMs ?? 1_000;
  const maxBackoff = opts.maxBackoffMs ?? 30_000;
  const random = opts.random ?? Math.random;

  const handlers = new Map<string, Set<(data: never) => void>>();
  const listeners = new Set<() => void>();

  let status: RealtimeStatus = "idle";
  let wantOpen = false;
  let socket: SocketLike | null = null;
  let attempt = 0;
  let awaitingPong = false;
  let pingTimer: unknown = null;
  let handshakeTimer: unknown = null;
  let retryTimer: unknown = null;

  function setStatus(next: RealtimeStatus) {
    if (status === next) return;
    status = next;
    listeners.forEach((l) => l());
  }

  function clearTimers() {
    if (pingTimer !== null) timers().clearInterval(pingTimer);
    if (handshakeTimer !== null) timers().clearTimeout(handshakeTimer);
    if (retryTimer !== null) timers().clearTimeout(retryTimer);
    pingTimer = handshakeTimer = retryTimer = null;
  }

  function emit(type: string, data: unknown) {
    for (const h of handlers.get(type) ?? []) {
      try {
        (h as (d: unknown) => void)(data);
      } catch {
        // One failing handler must not stop the others or the read loop.
      }
    }
  }

  function dispatch(raw: unknown) {
    if (typeof raw !== "string") return;
    let frame: unknown;
    try {
      frame = JSON.parse(raw);
    } catch {
      return;
    }
    if (typeof frame !== "object" || frame === null) return;
    const { type, data } = frame as { type?: unknown; data?: unknown };
    if (typeof type !== "string") return;
    if (type === "pong") {
      awaitingPong = false;
      return;
    }
    if (type === "ready") {
      if (status !== "authenticating") return;
      clearTimers();
      attempt = 0;
      awaitingPong = false;
      pingTimer = timers().setInterval(ping, pingMs);
      setStatus("ready");
    }
    if (serverEventTypes.has(type)) emit(type, data);
  }

  function ping() {
    if (!socket) return;
    if (awaitingPong) {
      // The last ping went unanswered for a whole interval: the link is dead.
      socket.close(4000, "ping timeout");
      return;
    }
    awaitingPong = true;
    socket.send(JSON.stringify({ type: "ping" }));
  }

  function open() {
    const token = opts.getToken();
    if (!token) {
      wantOpen = false;
      setStatus("closed");
      return;
    }
    const sock = opts.createSocket(opts.url);
    socket = sock;
    setStatus(attempt === 0 ? "connecting" : "reconnecting");
    sock.onopen = () => {
      if (socket !== sock) return;
      sock.send(JSON.stringify({ type: "auth", data: { access_token: token } }));
      setStatus("authenticating");
      handshakeTimer = timers().setTimeout(() => sock.close(4000, "handshake timeout"), handshakeMs);
    };
    sock.onmessage = asHandler<SocketEvents["message"]>((ev) => {
      if (socket === sock) dispatch(ev.data);
    });
    sock.onerror = () => {
      // A close event always follows; reconnecting is decided there.
    };
    sock.onclose = asHandler<SocketEvents["close"]>((ev) => {
      if (socket !== sock) return;
      socket = null;
      clearTimers();
      if (!wantOpen) {
        setStatus("closed");
      } else if (ev.code === closeUnauthorized && opts.onUnauthorized) {
        void recoverAuth(token);
      } else {
        scheduleReconnect();
      }
    });
  }

  async function recoverAuth(rejected: string) {
    setStatus("reconnecting");
    let refreshed = false;
    try {
      refreshed = await opts.onUnauthorized!(rejected);
    } catch {
      // A failed refresh is treated like a refused one below.
    }
    if (!wantOpen || socket) return; // closed or re-opened while refreshing
    if (!refreshed) {
      wantOpen = false;
      setStatus("closed");
    } else if (opts.getToken() === rejected) {
      scheduleReconnect(); // still the same token: do not hammer the hub
    } else {
      open();
    }
  }

  function scheduleReconnect() {
    const step = Math.min(maxBackoff, minBackoff * 2 ** attempt);
    attempt += 1;
    setStatus("reconnecting");
    // Jitter keeps clients that lost the same server from reconnecting together.
    retryTimer = timers().setTimeout(
      () => {
        retryTimer = null;
        if (wantOpen) open();
      },
      step * (0.5 + 0.5 * random()),
    );
  }

  function teardown() {
    clearTimers();
    const sock = socket;
    socket = null;
    sock?.close(1000);
  }

  return {
    connect() {
      if (wantOpen) return;
      wantOpen = true;
      attempt = 0;
      open();
    },
    close() {
      wantOpen = false;
      teardown();
      setStatus("closed");
    },
    reauth() {
      if (!wantOpen) return;
      teardown();
      attempt = 0;
      open();
    },
    on(type, handler) {
      let set = handlers.get(type);
      if (!set) handlers.set(type, (set = new Set()));
      set.add(handler as (data: never) => void);
      return () => {
        set.delete(handler as (data: never) => void);
      };
    },
    send(type, data) {
      if (!socket || status !== "ready") return false;
      socket.send(JSON.stringify({ type, data }));
      return true;
    },
    getStatus: () => status,
    subscribe(listener) {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
  };
}
