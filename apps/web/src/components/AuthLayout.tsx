import { Navigate, useLocation } from "react-router";
import type { ReactNode } from "react";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { useSessionState } from "../state/session";
import { Brand } from "./Brand";
import { LanguageSelect } from "./LanguageSelect";
import { ThemeToggle } from "./ThemeToggle";

/** Where to go after signing in: the page the visitor was sent away from, or home. */
export function useReturnTo(): string {
  const state = useLocation().state as { from?: { pathname: string; search?: string } } | null;
  return state?.from ? `${state.from.pathname}${state.from.search ?? ""}` : "/";
}

/** The frame of the sign-in and sign-up pages: a centred card with the language and theme switches under it. */
export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  const { signedIn } = useSessionState();
  const returnTo = useReturnTo();
  if (signedIn) return <Navigate to={returnTo} replace />;
  return (
    <main className="flex min-h-dvh flex-col items-center bg-muted/40 px-4 py-10 sm:justify-center">
      <div className="grid w-full max-w-sm gap-6">
        <Brand to={null} className="justify-center" />
        <Card className="gap-5 py-6 shadow-sm">
          <CardHeader>
            <h1 className="text-xl leading-tight font-semibold tracking-tight">{title}</h1>
          </CardHeader>
          <CardContent>{children}</CardContent>
        </Card>
        <footer className="flex items-center justify-center gap-1">
          <LanguageSelect />
          <ThemeToggle />
        </footer>
      </div>
    </main>
  );
}
