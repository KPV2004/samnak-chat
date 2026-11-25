import { useQueryClient } from "@tanstack/react-query";
import { Suspense, useEffect, useRef } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Toaster } from "@/components/ui/sonner";
import { cn } from "@/lib/utils";
import { AppSidebar } from "../components/AppSidebar";
import { NoWorkspace } from "../components/NoWorkspace";
import { LanguageSelect } from "../components/LanguageSelect";
import { QuickSwitch } from "../components/QuickSwitch";
import { RouteFallback } from "../components/RouteFallback";
import { TabBar } from "../components/TabBar";
import { ThemeToggle } from "../components/ThemeToggle";
import { VerifyEmailBanner } from "../components/VerifyEmailBanner";
import { NotificationBell } from "../notify/NotificationBell";
import { ChatProvider } from "../state/chat";
import { useT } from "../i18n/useT";
import { useLocale } from "../state/locale";
import { useMe, useSyncWorkspace, useWorkspaces } from "../state/queries";
import { useSessionState } from "../state/session";

export function Shell() {
  const t = useT();
  const { signedIn } = useSessionState();
  const location = useLocation();
  const queryClient = useQueryClient();
  const setLocale = useLocale((s) => s.setLocale);
  const me = useMe();
  const { query: workspacesQuery, workspaces, current } = useWorkspaces();
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

  // A user without a workspace gets one honest next step instead of pages that can only fail (BT-7).
  // The two workspace-free pages stay reachable: create one, or accept an invite into one.
  const workspaceFree = location.pathname.startsWith("/workspaces/new") || location.pathname.startsWith("/invite/");
  const noWorkspace = workspacesQuery.isSuccess && (workspaces?.length ?? 0) === 0 && !workspaceFree;

  const isChat = location.pathname.startsWith("/chat");

  return (
    <ChatProvider enabled={!noWorkspace}>
      <a
        href="#main"
        className="sr-only z-50 rounded-md bg-background px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t("ui.skipToContent")}
      </a>
      <div className="flex h-dvh bg-background">
        {/* One left column: 64px of icons at md, the full column from lg. Phones use the tab bar. */}
        <aside className="hidden w-16 shrink-0 border-r bg-sidebar md:flex lg:w-60">
          <AppSidebar />
        </aside>
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="flex h-12 shrink-0 items-center gap-3 border-b bg-card px-3 md:px-4">
            <h1 className="m-0 min-w-0 flex-1 truncate text-base font-semibold md:hidden">{current?.name ?? t("title")}</h1>
            <div className="hidden w-full max-w-80 md:block">
              <QuickSwitch />
            </div>
            <div className="ml-auto flex items-center gap-1">
              <NotificationBell />
              <div className="hidden md:block">
                <LanguageSelect />
              </div>
              <ThemeToggle />
            </div>
          </header>
          <div className={cn("min-h-0 flex-1", isChat ? "overflow-hidden" : "overflow-y-auto")}>
            {/* Content outside a Page (loading and gate states) gets the page frame; a Page brings its own padding. */}
            <main
              id="main"
              tabIndex={-1}
              className={cn(
                "outline-none",
                isChat ? "h-full" : "mx-auto w-full max-w-6xl p-4 md:p-6 has-[[data-page]]:p-0",
              )}
            >
              <VerifyEmailBanner />
              {noWorkspace ? (
                <NoWorkspace />
              ) : (
                <Suspense fallback={<RouteFallback />}>
                  <Outlet />
                </Suspense>
              )}
            </main>
          </div>
          <TabBar />
        </div>
      </div>
      <Toaster />
    </ChatProvider>
  );
}
