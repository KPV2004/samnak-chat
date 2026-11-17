import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { Link, NavLink, Navigate, Outlet, useLocation } from "react-router";
import { LanguageSelect } from "../components/LanguageSelect";
import { NotificationBell } from "../notify/NotificationBell";
import { TaskNavLink } from "../task/TaskNavLink";
import { ChatProvider } from "../state/chat";
import { useT } from "../i18n/useT";
import { useLocale } from "../state/locale";
import { useMe, useSyncWorkspace, useWorkspaces } from "../state/queries";
import { useSession, useSessionState } from "../state/session";

export function Shell() {
  const t = useT();
  const session = useSession();
  const { signedIn } = useSessionState();
  const location = useLocation();
  const queryClient = useQueryClient();
  const setLocale = useLocale((s) => s.setLocale);
  const me = useMe();
  const { workspaces, current } = useWorkspaces();
  const localeAppliedFor = useRef<string | null>(null);

  useSyncWorkspace();

  // Adopt the language saved on the profile once per sign-in; later changes in the header stay local.
  useEffect(() => {
    if (me.data && localeAppliedFor.current !== me.data.id) {
      localeAppliedFor.current = me.data.id;
      setLocale(me.data.locale);
    }
  }, [me.data, setLocale]);

  // Never show one user's cached data to the next.
  useEffect(() => {
    if (!signedIn) {
      localeAppliedFor.current = null;
      queryClient.clear();
    }
  }, [signedIn, queryClient]);

  if (!signedIn) return <Navigate to="/login" replace state={{ from: location }} />;

  return (
    <ChatProvider>
      <header className="shell-header">
        <Link to="/" className="brand">
          {t("title")}
        </Link>
        {workspaces && workspaces.length > 0 && (
          <label className="inline">
            {t("workspace.switch")}{" "}
            <select value={current?.id ?? ""} onChange={(e) => session.setWorkspaceId(e.target.value)}>
              {workspaces.map((w) => (
                <option key={w.id} value={w.id}>
                  {w.name}
                </option>
              ))}
            </select>
          </label>
        )}
        <Link to="/workspaces/new">{t("workspace.new")}</Link>
        <NotificationBell />
        <LanguageSelect />
        <button
          type="button"
          onClick={() => void session.logout(false)}
        >
          {t("signOut")}
        </button>
      </header>
      <nav aria-label={t("nav.label")} className="shell-nav">
        <NavLink to="/" end>
          {t("nav.home")}
        </NavLink>
        <NavLink to="/chat">{t("nav.chat")}</NavLink>
        <TaskNavLink />
        <NavLink to="/members">{t("nav.members")}</NavLink>
        <NavLink to="/invites">{t("nav.invites")}</NavLink>
        <NavLink to="/profile">{t("nav.profile")}</NavLink>
        <NavLink to="/devices">{t("nav.devices")}</NavLink>
      </nav>
      <main className={location.pathname.startsWith("/chat") ? "wide" : undefined}>
        <Outlet />
      </main>
    </ChatProvider>
  );
}
