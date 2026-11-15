import {
  createChatStore,
  createM2Api,
  createRealtimeClient,
  type ChatState,
  type ChatStore,
  type M2Api,
  type RealtimeClient,
  type RealtimeStatus,
  type SocketFactory,
} from "@samnak/core";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type ReactNode,
} from "react";
import { useMe } from "./queries";
import { useSession, useSessionState } from "./session";

export interface RealtimeEnvironment {
  url: string;
  createSocket: SocketFactory;
}

function browserEnvironment(): RealtimeEnvironment {
  const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
  return {
    url: `${scheme}//${window.location.host}/api/v1/ws`,
    createSocket: (url) => new WebSocket(url),
  };
}

const EnvironmentContext = createContext<RealtimeEnvironment | null>(null);

/** Lets tests replace the WebSocket. The app itself uses the browser's. */
export function RealtimeEnvironmentProvider({ value, children }: { value: RealtimeEnvironment; children: ReactNode }) {
  return <EnvironmentContext value={value}>{children}</EnvironmentContext>;
}

export type ChannelsStatus = "loading" | "ready" | "error";

export interface ChatRuntime {
  api: M2Api;
  realtime: RealtimeClient;
  store: ChatStore;
  userId: string;
  channelsStatus: ChannelsStatus;
  /** Loads the joined channels again, e.g. after joining one. */
  reloadChannels(): Promise<void>;
}

const ChatContext = createContext<ChatRuntime | null>(null);

interface Core {
  api: M2Api;
  realtime: RealtimeClient;
  store: ChatStore;
  userId: string;
}

/** What the last load of the channel list said, and for which store, workspace and attempt. */
interface Loaded {
  core: Core;
  workspaceId: string | null;
  attempt: number;
  status: "ready" | "error";
}

/**
 * Owns the realtime connection and the chat store of the signed-in user: one
 * socket per session, re-authenticated when the access token is refreshed, and
 * the store is emptied and reloaded when the workspace changes.
 */
export function ChatProvider({ children }: { children: ReactNode }) {
  const session = useSession();
  const { signedIn, workspaceId } = useSessionState();
  const userId = useMe().data?.id;
  const environment = useContext(EnvironmentContext);
  const [attempt, setAttempt] = useState(0);
  const [loaded, setLoaded] = useState<Loaded | null>(null);

  // Creating the client and the store connects nothing; the effect below does.
  const core = useMemo<Core | null>(() => {
    if (!signedIn || !userId) return null;
    const env = environment ?? browserEnvironment();
    const api = createM2Api(session.client);
    const realtime = createRealtimeClient({
      url: env.url,
      createSocket: env.createSocket,
      getToken: () => session.getAccessToken(),
      onUnauthorized: (rejected) => session.refreshAccessToken(rejected),
    });
    const store = createChatStore({
      api,
      realtime,
      userId,
      getWorkspaceId: () => session.getState().workspaceId,
    });
    return { api, realtime, store, userId };
  }, [session, environment, signedIn, userId]);

  useEffect(() => {
    if (!core) return;
    const offToken = session.onTokenChange(() => core.realtime.reauth());
    core.realtime.connect();
    return () => {
      offToken();
      core.realtime.close();
    };
  }, [core, session]);

  // Another workspace has other channels: forget these before loading those.
  useEffect(() => {
    core?.store.reset();
  }, [core, workspaceId]);

  useEffect(() => {
    if (!core || !workspaceId) return;
    let current = true;
    const done = (status: Loaded["status"]) => current && setLoaded({ core, workspaceId, attempt, status });
    core.store.loadChannels().then(
      () => done("ready"),
      () => done("error"),
    );
    return () => {
      current = false;
    };
  }, [core, workspaceId, attempt]);

  const reloadChannels = useCallback(async () => setAttempt((n) => n + 1), []);

  const channelsStatus: ChannelsStatus =
    loaded && loaded.core === core && loaded.workspaceId === workspaceId && loaded.attempt === attempt
      ? loaded.status
      : "loading";

  const runtime = useMemo<ChatRuntime | null>(
    () => (core ? { ...core, channelsStatus, reloadChannels } : null),
    [core, channelsStatus, reloadChannels],
  );
  return <ChatContext value={runtime}>{children}</ChatContext>;
}

/** The chat runtime, or null before the user is known (and while signed out). */
export function useChat(): ChatRuntime | null {
  return useContext(ChatContext);
}

const emptyState: ChatState = { channels: {}, workspaceUnread: {}, activeChannelId: null };
const never = () => () => {};

/** The store's current snapshot; an empty one while there is no runtime. */
export function useChatState(): ChatState {
  const chat = useChat();
  return useSyncExternalStore(chat?.store.subscribe ?? never, chat?.store.getState ?? (() => emptyState));
}

const idle: RealtimeStatus = "idle";

export function useRealtimeStatus(): RealtimeStatus {
  const chat = useChat();
  return useSyncExternalStore(chat?.realtime.subscribe ?? never, chat?.realtime.getStatus ?? (() => idle));
}
