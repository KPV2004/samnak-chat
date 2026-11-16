// M3 (notify) types and calls. The notify endpoints are not in the server's
// OpenAPI document until checkpoint D, so these types are hand-written, the way
// M2 started; `npm run gen:api` then replaces them like the rest.
import type { ApiClient } from "./client";

/** One row of `GET /notify/notifications`, newest first. */
export interface NotificationItem {
  id: string;
  /** "<module>.<name>", e.g. "chat.mention". */
  kind: string;
  /** Filled by the source module: the sender and where it happened. */
  title: string;
  /** A plain-text excerpt. */
  body: string;
  /** Web path to open, e.g. "/chat/{channel_id}". */
  link: string;
  /** The source module's details, e.g. chat's `channel_id`, `message_id`, `seq`. */
  data: Record<string, unknown>;
  read_at: string | null;
  created_at: string;
}

export interface NotificationPage {
  items: NotificationItem[];
  has_more: boolean;
}

export interface UnreadNotificationCount {
  count: number;
}

export interface ListNotificationsParams {
  /** Only the unread ones. */
  unread?: boolean;
  /** Cursor: the id of the last item of the previous page. */
  before?: string;
  /** 1 to 50; the server defaults to 20. */
  limit?: number;
}

export interface M3Api {
  listNotifications(params?: ListNotificationsParams): Promise<NotificationPage>;
  unreadNotificationCount(): Promise<UnreadNotificationCount>;
  readNotification(id: string): Promise<void>;
  readAllNotifications(): Promise<void>;
}

const base = "/api/v1";
const enc = encodeURIComponent;

export function createM3Api(client: ApiClient): M3Api {
  return {
    listNotifications(params = {}) {
      const qs = Object.entries({ unread: params.unread, before: params.before, limit: params.limit })
        .filter(([, value]) => value !== undefined)
        .map(([key, value]) => `${key}=${enc(String(value))}`)
        .join("&");
      return client.request(`${base}/notify/notifications${qs ? `?${qs}` : ""}`);
    },
    unreadNotificationCount: () => client.request(`${base}/notify/unread-count`),
    readNotification: (id) => client.request(`${base}/notify/notifications/${enc(id)}/read`, { method: "POST" }),
    readAllNotifications: () => client.request(`${base}/notify/read-all`, { method: "POST" }),
  };
}
