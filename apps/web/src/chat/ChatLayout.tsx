import { Suspense } from "react";
import { Outlet } from "react-router";
import { RouteFallback } from "../components/RouteFallback";
import { useT } from "../i18n/useT";
import { useRealtimeStatus } from "../state/chat";
import { ConversationList } from "./ConversationList";
import { useWide } from "./useWide";

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
      {/* A channel fills the pane (it has its own header and composer); other pages get the padding. */}
      <section className="row-start-2 min-h-0 min-w-0 overflow-y-auto p-4 has-[[data-channel]]:overflow-hidden has-[[data-channel]]:p-0">
        <Suspense fallback={<RouteFallback />}>
          <Outlet />
        </Suspense>
      </section>
    </div>
  );
}

export function ChatHome() {
  const t = useT();
  const wide = useWide();
  // Below lg the left column only shows icons, so the list lives here, as on a phone.
  if (!wide) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <ConversationList />
      </div>
    );
  }
  return <p className="p-4 text-muted-foreground">{t("chat.empty")}</p>;
}
