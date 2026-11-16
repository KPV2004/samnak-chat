import type { NotificationItem } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatMessageTime } from "../lib/format";
import { useSession } from "../state/session";
import { notificationLabel, notificationLink } from "./notificationText";
import { useNotificationStore } from "./useNotificationStore";
import "./notify.css";

const maxShown = 99;

/**
 * The bell in the shell header (NOTI-03, NOTI-04). It is hidden when `/meta`
 * does not list notify, so a server with the module turned off looks as before.
 */
export function NotificationBell() {
  const t = useT();
  const navigate = useNavigate();
  const session = useSession();
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => session.client.getMeta() });
  const enabled = meta.data?.modules.includes("notify") ?? false;
  const { store, state } = useNotificationStore(enabled);
  const [open, setOpen] = useState(false);

  const count = state.unreadCount;
  const label =
    count === 0
      ? t("notify.bell")
      : count > maxShown
        ? t("notify.bellMany")
        : t("notify.bellUnread", { n: count });

  if (!enabled) return null;

  function toggle() {
    const next = !open;
    setOpen(next);
    if (next) void store?.load();
  }

  function openItem(notification: NotificationItem) {
    void store?.markRead(notification.id);
    const link = notificationLink(notification);
    setOpen(false);
    if (link) navigate(link);
  }

  return (
    <div
      className="notify"
      onKeyDown={(e) => {
        if (e.key === "Escape") setOpen(false);
      }}
    >
      <button type="button" className="notify-bell" aria-expanded={open} onClick={toggle}>
        <span aria-hidden="true">🔔</span>
        {count > 0 && (
          <span className="notify-count" aria-hidden="true">
            {count > maxShown ? `${maxShown}+` : count}
          </span>
        )}
        <span className="visually-hidden">{label}</span>
      </button>
      {open && (
        <section className="notify-panel" aria-label={t("notify.panel")}>
          <div className="notify-panel-head">
            <h2>{t("notify.panel")}</h2>
            <button type="button" onClick={() => void store?.markAllRead()} disabled={count === 0}>
              {t("notify.markAll")}
            </button>
          </div>
          {state.status === "loading" && <p role="status">{t("loading")}</p>}
          {state.status === "error" && (
            <p role="alert">
              {t("notify.loadError")}{" "}
              <button type="button" onClick={() => void store?.load()}>
                {t("retry")}
              </button>
            </p>
          )}
          {state.status === "ready" && state.items.length === 0 && <p className="muted">{t("notify.empty")}</p>}
          {state.items.length > 0 && (
            <ul className="notify-list">
              {state.items.map((notification) => (
                <NotificationRow key={notification.id} notification={notification} onOpen={openItem} />
              ))}
            </ul>
          )}
          {state.hasMore && (
            <button type="button" className="notify-more" onClick={() => void store?.loadMore()}>
              {t("loadMore")}
            </button>
          )}
        </section>
      )}
    </div>
  );
}

function NotificationRow({
  notification,
  onOpen,
}: {
  notification: NotificationItem;
  onOpen: (notification: NotificationItem) => void;
}) {
  const t = useT();
  const locale = useCurrentLocale();
  const label = notificationLabel(notification.kind, t);
  return (
    <li className={notification.read_at ? "notify-item read" : "notify-item"}>
      <button type="button" className="notify-open" onClick={() => onOpen(notification)}>
        <span className="notify-line">
          {label && <span className="notify-kind">{label}</span>}
          <span className="notify-title">{notification.title}</span>
          <time className="notify-time" dateTime={notification.created_at}>
            {formatMessageTime(notification.created_at, locale)}
          </time>
        </span>
        {notification.body && <span className="notify-body">{notification.body}</span>}
        {!notification.read_at && <span className="visually-hidden"> ({t("notify.unread")})</span>}
      </button>
    </li>
  );
}
