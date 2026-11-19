// M3 (notify) types come from the server's OpenAPI document (npm run gen:api);
// this file re-exports them under stable names and holds the calls, the way
// m2.ts does.
import type { ApiClient } from "./client";
import type { components } from "./schema";

type Schemas = components["schemas"];

/** One row of `GET /notify/notifications`, newest first. */
export type NotificationItem = Schemas["Notification"];
export type NotificationPage = Schemas["NotificationsOutputBody"];
export type UnreadNotificationCount = Schemas["UnreadCountOutputBody"];
/** My notification settings in one workspace (NOTI-07). Times are HH:MM; both or neither of the DND pair. */
export type NotifyPreferences = Schemas["Preferences"];

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
  /** Defaults when never saved: every kind on, no channel muted, no do-not-disturb. */
  getPreferences(): Promise<NotifyPreferences>;
  /** Replaces the whole settings object; send every field, not a patch. */
  setPreferences(prefs: NotifyPreferences): Promise<NotifyPreferences>;
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
    getPreferences: () => client.request(`${base}/notify/preferences`),
    setPreferences: (prefs) =>
      client.request(`${base}/notify/preferences`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(prefs),
      }),
  };
}
