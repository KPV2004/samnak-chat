import type { NotificationItem } from "@samnak/core";
import { useQuery } from "@tanstack/react-query";
import { Bell, BellOff } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { EmptyState } from "../components/EmptyState";
import { useCurrentLocale, useT } from "../i18n/useT";
import { formatMessageTime } from "../lib/format";
import { useSession } from "../state/session";
import { notificationLabel, notificationLink, notificationTitle } from "./notificationText";
import { useNotificationToasts } from "./useNotificationToasts";
import { useNotificationStore } from "./useNotificationStore";

const maxShown = 99;

/**
 * The bell in the shell header (NOTI-03, NOTI-04). It opens a popover with the newest
 * notifications. It is hidden when `/meta` does not list notify, so a server with the
 * module turned off looks as before.
 */
export function NotificationBell() {
  const t = useT();
  const navigate = useNavigate();
  const session = useSession();
  const meta = useQuery({ queryKey: ["meta"], queryFn: () => session.client.getMeta() });
  const enabled = meta.data?.modules.includes("notify") ?? false;
  const { store, state } = useNotificationStore(enabled);
  useNotificationToasts(enabled);
  const [open, setOpen] = useState(false);

  const count = state.unreadCount;
  const label =
    count === 0
      ? t("notify.bell")
      : count > maxShown
        ? t("notify.bellMany")
        : t("notify.bellUnread", { n: count });

  if (!enabled) return null;

  function changeOpen(next: boolean) {
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
    <Popover open={open} onOpenChange={changeOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="ghost" size="icon-sm" className="relative">
          <Bell aria-hidden="true" />
          {count > 0 && (
            <span
              aria-hidden="true"
              className="absolute -top-1 -right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary px-1 text-[10px] font-semibold text-primary-foreground"
            >
              {count > maxShown ? `${maxShown}+` : count}
            </span>
          )}
          <span className="sr-only">{label}</span>
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-96 max-w-[calc(100vw-2rem)] p-0">
        <section aria-label={t("notify.panel")} className="flex flex-col">
          <div className="flex items-center justify-between gap-2 border-b px-4 py-3">
            <h2 className="m-0 text-base font-semibold">{t("notify.panel")}</h2>
            <Button type="button" variant="ghost" size="sm" onClick={() => void store?.markAllRead()} disabled={count === 0}>
              {t("notify.markAll")}
            </Button>
          </div>
          <div className="max-h-[60vh] overflow-y-auto">
            {state.status === "loading" && (
              <div role="status" className="grid gap-2 p-4">
                <span className="sr-only">{t("loading")}</span>
                <Skeleton className="h-12 w-full" />
                <Skeleton className="h-12 w-full" />
              </div>
            )}
            {state.status === "error" && (
              <div role="alert" className="flex flex-wrap items-center gap-2 p-4 text-sm text-destructive">
                <p className="m-0">{t("notify.loadError")}</p>
                <Button type="button" variant="outline" size="sm" onClick={() => void store?.load()}>
                  {t("retry")}
                </Button>
              </div>
            )}
            {state.status === "ready" && state.items.length === 0 && (
              <EmptyState icon={BellOff} title={t("notify.empty")} className="m-4 border-0" />
            )}
            {state.items.length > 0 && (
              <ul className="divide-y">
                {state.items.map((notification) => (
                  <NotificationRow key={notification.id} notification={notification} onOpen={openItem} />
                ))}
              </ul>
            )}
          </div>
          {state.hasMore && (
            <div className="border-t p-2">
              <Button type="button" variant="ghost" size="sm" className="w-full" onClick={() => void store?.loadMore()}>
                {t("loadMore")}
              </Button>
            </div>
          )}
        </section>
      </PopoverContent>
    </Popover>
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
  const read = Boolean(notification.read_at);
  return (
    <li>
      <button
        type="button"
        onClick={() => onOpen(notification)}
        className="grid w-full gap-1 px-4 py-3 text-left outline-none hover:bg-accent focus-visible:bg-accent focus-visible:ring-[3px] focus-visible:ring-inset focus-visible:ring-ring/50"
      >
        <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs">
          {!read && <span aria-hidden="true" className="size-1.5 rounded-full bg-primary" />}
          {label && <span className="text-muted-foreground">{label}</span>}
          <time dateTime={notification.created_at} className="ml-auto text-muted-foreground">
            {formatMessageTime(notification.created_at, locale)}
          </time>
        </span>
        <span className={cn("text-sm", read ? "text-muted-foreground" : "font-medium")}>{notificationTitle(notification, t)}</span>
        {notification.body && <span className="text-sm text-muted-foreground">{notification.body}</span>}
        {!read && <span className="sr-only"> ({t("notify.unread")})</span>}
      </button>
    </li>
  );
}
