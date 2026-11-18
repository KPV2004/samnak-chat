import { Suspense } from "react";
import { Outlet } from "react-router";
import { RouteFallback } from "../components/RouteFallback";
import { useT } from "../i18n/useT";
import { useRealtimeStatus } from "../state/chat";

/** The chat area. The channel list is in the app sidebar; this holds the reconnect banner and the open page. */
export function ChatLayout() {
  const t = useT();
  const status = useRealtimeStatus();
  return (
    <div className="grid h-full grid-rows-[auto_minmax(0,1fr)]">
      {status === "reconnecting" && (
        <p className="m-0 border-b bg-muted px-4 py-1.5 text-sm text-muted-foreground" role="status">
          {t("chat.reconnecting")}
        </p>
      )}
      <section className="chat-pane">
        <Suspense fallback={<RouteFallback />}>
          <Outlet />
        </Suspense>
      </section>
    </div>
  );
}

export function ChatHome() {
  const t = useT();
  return <p className="muted chat-empty">{t("chat.empty")}</p>;
}
