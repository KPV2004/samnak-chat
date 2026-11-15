import { Outlet } from "react-router";
import { useT } from "../i18n/useT";
import { useRealtimeStatus } from "../state/chat";
import { Sidebar } from "./Sidebar";

export function ChatLayout() {
  const t = useT();
  const status = useRealtimeStatus();
  return (
    <div className="chat">
      {status === "reconnecting" && (
        <p className="chat-banner" role="status">
          {t("chat.reconnecting")}
        </p>
      )}
      <Sidebar />
      <section className="chat-pane">
        <Outlet />
      </section>
    </div>
  );
}

export function ChatHome() {
  const t = useT();
  return <p className="muted chat-empty">{t("chat.empty")}</p>;
}
