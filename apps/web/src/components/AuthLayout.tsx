import { Navigate, useLocation } from "react-router";
import type { ReactNode } from "react";
import { useT } from "../i18n/useT";
import { useSessionState } from "../state/session";
import { LanguageSelect } from "./LanguageSelect";

/** Where to go after signing in: the page the visitor was sent away from, or home. */
export function useReturnTo(): string {
  const state = useLocation().state as { from?: { pathname: string; search?: string } } | null;
  return state?.from ? `${state.from.pathname}${state.from.search ?? ""}` : "/";
}

export function AuthLayout({ title, children }: { title: string; children: ReactNode }) {
  const t = useT();
  const { signedIn } = useSessionState();
  const returnTo = useReturnTo();
  if (signedIn) return <Navigate to={returnTo} replace />;
  return (
    <main className="auth">
      <p className="brand">{t("title")}</p>
      <h1>{title}</h1>
      {children}
      <footer>
        <LanguageSelect />
      </footer>
    </main>
  );
}
