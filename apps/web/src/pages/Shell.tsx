import { useQueryClient } from "@tanstack/react-query";
import { Menu } from "lucide-react";
import { Suspense, useEffect, useRef, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle, SheetTrigger } from "@/components/ui/sheet";
import { Toaster } from "@/components/ui/sonner";
import { cn } from "@/lib/utils";
import { AppSidebar } from "../components/AppSidebar";
import { Brand } from "../components/Brand";
import { LanguageSelect } from "../components/LanguageSelect";
import { RouteFallback } from "../components/RouteFallback";
import { ThemeToggle } from "../components/ThemeToggle";
import { NotificationBell } from "../notify/NotificationBell";
import { ChatProvider } from "../state/chat";
import { useT } from "../i18n/useT";
import { useLocale } from "../state/locale";
import { useMe, useSyncWorkspace } from "../state/queries";
import { useSessionState } from "../state/session";

export function Shell() {
  const t = useT();
  const { signedIn } = useSessionState();
  const location = useLocation();
  const queryClient = useQueryClient();
  const setLocale = useLocale((s) => s.setLocale);
  const me = useMe();
  const [menuOpen, setMenuOpen] = useState(false);
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

  const isChat = location.pathname.startsWith("/chat");

  return (
    <ChatProvider>
      <a
        href="#main"
        className="sr-only z-50 rounded-md bg-background px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t("ui.skipToContent")}
      </a>
      <div className="flex h-dvh bg-background">
        <aside className="hidden w-64 shrink-0 border-r border-sidebar-border md:block">
          <AppSidebar />
        </aside>
        <div className="flex min-w-0 flex-1 flex-col">
          <header className="relative z-10 flex h-12 shrink-0 items-center gap-2 border-b px-3 md:px-4">
            <Sheet open={menuOpen} onOpenChange={setMenuOpen}>
              <SheetTrigger asChild>
                <Button type="button" variant="ghost" size="icon-sm" className="md:hidden" aria-label={t("ui.openMenu")}>
                  <Menu />
                </Button>
              </SheetTrigger>
              <SheetContent
                side="left"
                className="w-72 gap-0 p-0"
                aria-describedby={undefined}
                // Focus the panel rather than the first control, which would open with a focus ring on the workspace switch.
                onOpenAutoFocus={(e) => {
                  e.preventDefault();
                  (e.currentTarget as HTMLElement | null)?.focus();
                }}
              >
                <SheetTitle className="sr-only">{t("title")}</SheetTitle>
                <AppSidebar onNavigate={() => setMenuOpen(false)} />
              </SheetContent>
            </Sheet>
            <Brand className="md:hidden" />
            <div className="ml-auto flex items-center gap-1">
              <NotificationBell />
              <LanguageSelect />
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
              <Suspense fallback={<RouteFallback />}>
                <Outlet />
              </Suspense>
            </main>
          </div>
        </div>
      </div>
      <Toaster />
    </ChatProvider>
  );
}
